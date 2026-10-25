// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/collector/transport"

	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

type targetRuntime struct {
	config TargetConfig
	client EvidenceClient
}

// ClaimedSource resolves host workflow claims into observability source
// facts. It owns target validation, normalization, envelope construction,
// and source-local failure classification only: admission, projection,
// persistence, claim mutation, and fencing stay core-owned. Every Collect
// call enforces the core-issued producer grant first and fails closed
// without one.
type ClaimedSource struct {
	collectorInstanceID string
	targets             map[string]targetRuntime
	now                 func() time.Time
	recorder            MetricsRecorder
}

// NewClaimedSource validates configuration and builds a claim-driven Grafana
// source. Disabled targets are ignored before client construction.
func NewClaimedSource(config SourceConfig) (*ClaimedSource, error) {
	collectorID := strings.TrimSpace(config.CollectorInstanceID)
	if collectorID == "" {
		return nil, fmt.Errorf("grafana collector instance ID is required")
	}
	factory := config.ClientFactory
	if factory == nil {
		factory = defaultClientFactory
	}
	targets := make(map[string]targetRuntime, len(config.Targets))
	for i, target := range config.Targets {
		if !target.Enabled {
			continue
		}
		validated, err := validateTarget(target)
		if err != nil {
			return nil, fmt.Errorf("target %d: %w", i, err)
		}
		if _, exists := targets[validated.ScopeID]; exists {
			return nil, fmt.Errorf("duplicate grafana target scope_id %q", validated.ScopeID)
		}
		client, err := factory(validated)
		if err != nil {
			return nil, fmt.Errorf("target %d client: %w", i, err)
		}
		targets[validated.ScopeID] = targetRuntime{config: validated, client: client}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one enabled grafana target is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &ClaimedSource{
		collectorInstanceID: collectorID,
		targets:             targets,
		now:                 now,
		recorder:            config.Metrics,
	}, nil
}

// Collect binds one host claim to its configured target, fetches bounded
// live metadata, and returns a collector-sdk/v1alpha1 Result. The grant is
// enforced at every emission: without a usable core-issued grant Collect
// fails closed before any provider contact. Provider failures return a
// classified transport.ProviderFailure for host claim disposition; the
// producer never decides retryable-vs-terminal claim fate itself.
func (s *ClaimedSource) Collect(
	ctx context.Context,
	claim collector.Claim,
	g grant.Grant,
) (collector.Result, error) {
	if err := grant.Check(g, time.Now()); err != nil {
		return collector.Result{}, err
	}
	if strings.TrimSpace(claim.InstanceID) != s.collectorInstanceID {
		return collector.Result{}, fmt.Errorf(
			"grafana work item collector_instance_id %q does not match source %q",
			claim.InstanceID,
			s.collectorInstanceID,
		)
	}
	if claim.CollectorKind != "" && claim.CollectorKind != CollectorKind {
		return collector.Result{}, fmt.Errorf("grafana source cannot collect %q work items", claim.CollectorKind)
	}
	// Strict source_system binding: the SDK host validator requires every
	// fact's source_ref.source_system to equal the claim's, and this
	// producer only emits grafana evidence. A foreign or blank value can
	// never be admitted, so it fails closed here instead of at commit.
	if claim.SourceSystem != CollectorKind {
		return collector.Result{}, fmt.Errorf("grafana source cannot collect %q source_system claims", claim.SourceSystem)
	}
	if strings.TrimSpace(claim.GenerationID) == "" {
		return collector.Result{}, fmt.Errorf("grafana work item generation_id is required")
	}
	target, ok := s.targets[strings.TrimSpace(claim.Scope.ID)]
	if !ok {
		err := fmt.Errorf("grafana target scope_id is not configured")
		return collector.Result{}, transport.NewProviderFailure("grafana", transport.FailureRetryable, false, err)
	}

	startedAt := time.Now()
	result, err := target.client.CollectObservedMetadata(ctx, target.config)
	if err != nil {
		failure := classifiedProviderFailure(err)
		s.recordFetch(target.config, failure.FailureClass(), startedAt)
		s.recordRateLimit(target.config, failure)
		s.recordRetries(target.config, result.Stats.Retries)
		return collector.Result{}, failure
	}

	observedAt := result.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	facts, err := s.envelopes(claim, target.config, result, observedAt)
	if err != nil {
		return collector.Result{}, err
	}
	s.recordFacts(target.config, facts)
	s.recordRedactions(target.config, result.Stats.Redactions)
	s.recordRateLimits(target.config, result.Stats.RateLimits)
	s.recordRetries(target.config, result.Stats.Retries)
	s.recordFetch(target.config, "success", startedAt)
	return s.result(claim, observedAt, result.Stats, len(result.Warnings), facts), nil
}

// result assembles the wire Result. A partial or truncated read stays a
// partial Result with a warning status so the host never mistakes bounded
// evidence for complete evidence; the SDK validator requires that status.
func (s *ClaimedSource) result(
	claim collector.Claim,
	observedAt time.Time,
	stats CollectionStats,
	warningCount int,
	facts []collector.Fact,
) collector.Result {
	state := collector.ResultComplete
	var statuses []collector.Status
	if stats.Partial || stats.Truncated {
		state = collector.ResultPartial
		statuses = []collector.Status{{
			Class:        collector.StatusWarning,
			Partial:      true,
			WarningCount: warningCount,
			FactCount:    len(facts),
		}}
	}
	return collector.Result{
		ProtocolVersion: collector.ProtocolVersionV1Alpha1,
		State:           state,
		Claim:           claim,
		Generation: collector.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    observedAt,
			FreshnessHint: "grafana_observed_metadata",
		},
		Facts:    facts,
		Statuses: statuses,
	}
}

func defaultClientFactory(target TargetConfig) (EvidenceClient, error) {
	return NewHTTPClient(HTTPClientConfig{
		BaseURL: target.BaseURL,
		Token:   target.Token,
	})
}

func classifiedProviderFailure(err error) ProviderFailure {
	return transport.ClassifyProviderFailure("grafana", err, transport.StatusPolicy{
		AuthDeniedClass: transport.FailureAuthDenied,
		NotFoundClass:   transport.FailureTerminal,
	}, transport.FailureRetryable)
}

func validateTarget(target TargetConfig) (TargetConfig, error) {
	target.Provider = strings.TrimSpace(target.Provider)
	if target.Provider == "" {
		target.Provider = ProviderGrafana
	}
	if target.Provider != ProviderGrafana {
		return TargetConfig{}, fmt.Errorf("unsupported grafana provider %q", target.Provider)
	}
	target.ScopeID = strings.TrimSpace(target.ScopeID)
	target.InstanceID = strings.TrimSpace(target.InstanceID)
	target.BaseURL = strings.TrimRight(strings.TrimSpace(target.BaseURL), "/")
	target.Token = strings.TrimSpace(target.Token)
	if target.ScopeID == "" {
		return TargetConfig{}, fmt.Errorf("scope_id is required")
	}
	if target.InstanceID == "" {
		return TargetConfig{}, fmt.Errorf("instance_id is required")
	}
	if target.BaseURL == "" {
		return TargetConfig{}, fmt.Errorf("base_url is required")
	}
	if target.Token == "" {
		return TargetConfig{}, fmt.Errorf("token is required")
	}
	if target.ResourceLimit < 0 || target.ResourceLimit > maxResourceLimit {
		return TargetConfig{}, fmt.Errorf("resource_limit must be between 0 and %d", maxResourceLimit)
	}
	if target.ResourceLimit == 0 {
		target.ResourceLimit = defaultResourceLimit
	}
	return target, nil
}

func (s *ClaimedSource) envelopes(
	claim collector.Claim,
	target TargetConfig,
	result CollectionResult,
	observedAt time.Time,
) ([]collector.Fact, error) {
	ctx := EnvelopeContext{
		ScopeID:             target.ScopeID,
		GenerationID:        claim.GenerationID,
		CollectorInstanceID: s.collectorInstanceID,
		ObservedAt:          observedAt,
		SourceURI:           target.BaseURL,
		SourceInstanceID:    target.InstanceID,
	}
	stats := result.Stats
	stats.Resources = len(result.Resources)
	stats.Rules = len(result.Rules)
	stats.Warnings = len(result.Warnings)
	envs := make([]collector.Fact, 0, 1+len(result.Resources)+len(result.Rules)+len(result.Warnings))
	source, err := NewSourceInstanceEnvelope(ctx, stats)
	if err != nil {
		return nil, err
	}
	envs = append(envs, source)
	for _, resource := range result.Resources {
		env, err := NewObservedResourceEnvelope(ctx, resource)
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}
	for _, rule := range result.Rules {
		env, err := NewObservedRuleEnvelope(ctx, rule)
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}
	for _, warning := range result.Warnings {
		env, err := NewCoverageWarningEnvelope(ctx, warning)
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}
	return envs, nil
}
