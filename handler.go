// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"errors"
	"strings"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/collector/transport"

	"github.com/eshu-hq/eshu-collector-grafana/grafana"
	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

// FailureGrantDenied marks a claim terminal-failed for lack of a usable
// core-issued producer grant. The host delivers the grant with the request;
// an unwired host gets this failure with zero facts, never evidence.
const FailureGrantDenied = "grant_denied"

// CollectorInstanceIDEnv names the environment variable carrying the
// collector instance this satellite is bound to. The binding comes from the
// host environment — never from the claim — so a claim addressed to another
// instance terminal-fails instead of collecting under a borrowed identity.
// An unset variable fails the claim closed: collecting without an instance
// binding would emit facts the host cannot attribute.
const CollectorInstanceIDEnv = "ESHU_GRAFANA_COLLECTOR_INSTANCE_ID"

// Handler runs one claim. The zero value is usable: it reads the process
// environment, the wall clock, and the real Grafana HTTP client.
type Handler struct {
	// Getenv resolves credential environment variables; nil selects an
	// empty lookup at the call site in cmd/collector.
	Getenv func(string) string
	// Now supplies the clock; nil selects time.Now.
	Now func() time.Time
	// NewClient builds the evidence client; nil selects the HTTP client.
	NewClient grafana.ClientFactory
}

func (h Handler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

func (h Handler) getenv(key string) string {
	if h.Getenv != nil {
		return h.Getenv(key)
	}
	return ""
}

func (h Handler) newClient(target grafana.TargetConfig) (grafana.EvidenceClient, error) {
	if h.NewClient != nil {
		return h.NewClient(target)
	}
	return grafana.NewHTTPClient(grafana.HTTPClientConfig{BaseURL: target.BaseURL, Token: target.Token})
}

// Handle collects the Grafana target named by the claim and returns the SDK
// result the host validates and commits. The host-delivered grant is
// enforced at every emission through the claim-driven source: without a
// usable grant the claim ends terminal with zero facts. It never returns
// facts together with a failure state, never lets a credential into a
// result, and echoes the claim unchanged so the host's identity check
// passes.
func (h Handler) Handle(ctx context.Context, request Request) sdk.Result {
	started := h.now()
	claim := request.Claim
	if claim.CollectorKind != "" && claim.CollectorKind != CollectorKind {
		return h.terminalResult(claim, started, grafana.FailureTerminal)
	}
	settings, err := ParseSettings(request.Config, claim.Scope.ID, h.getenv)
	if err != nil {
		// A claim naming a scope the config no longer carries is likely
		// stale — the host has not caught up — so it releases the claim
		// for a bounded retry instead of terminal-failing it.
		if strings.Contains(err.Error(), "is not configured") {
			return h.retryableResult(claim, started, grafana.FailureRetryable)
		}
		return h.terminalResult(claim, started, grafana.FailureTerminal)
	}
	var target *grafana.TargetConfig
	for i := range settings.Targets {
		if settings.Targets[i].ScopeID == strings.TrimSpace(claim.Scope.ID) {
			target = &settings.Targets[i]
			break
		}
	}
	if target == nil {
		return h.retryableResult(claim, started, grafana.FailureRetryable)
	}
	instanceID := strings.TrimSpace(h.getenv(CollectorInstanceIDEnv))
	if instanceID == "" {
		return h.terminalResult(claim, started, grafana.FailureTerminal)
	}
	source, err := grafana.NewClaimedSource(grafana.SourceConfig{
		CollectorInstanceID: instanceID,
		Targets:             settings.Targets,
		ClientFactory:       h.newClient,
		Now:                 h.Now,
		Metrics:             grafana.DiscardMetrics(),
	})
	if err != nil {
		return h.terminalResult(claim, started, grafana.FailureTerminal)
	}
	collected, err := source.Collect(ctx, claim, request.Grant)
	if err != nil {
		return h.collectErrorResult(claim, started, err)
	}
	return collected
}

// collectErrorResult maps a Collect failure onto the host retry contract. A
// grant denial is permanent — the host must deliver a grant, not retry — so
// it ends terminal with zero facts. Provider failures keep the classified
// retry contract.
func (h Handler) collectErrorResult(claim sdk.Claim, started time.Time, err error) sdk.Result {
	if errors.Is(err, grant.ErrDenied) {
		return h.terminalResult(claim, started, FailureGrantDenied)
	}
	var failure transport.ProviderFailure
	if errors.As(err, &failure) {
		if failure.TerminalFailure() {
			return h.terminalResult(claim, started, failure.FailureClass())
		}
		return h.retryableResult(claim, started, failure.FailureClass())
	}
	return h.terminalResult(claim, started, grafana.FailureTerminal)
}

func (h Handler) terminalResult(claim sdk.Claim, started time.Time, class string) sdk.Result {
	return sdk.Result{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		State:           sdk.ResultTerminal,
		Claim:           claim,
		Generation: sdk.Generation{
			ID:            claim.GenerationID,
			ObservedAt:    started,
			FreshnessHint: "grafana_observed_metadata",
		},
		Statuses: []sdk.Status{{
			Class:           sdk.StatusFailure,
			FailureClass:    class,
			SourceLatencyMS: latencyMillis(h.now().Sub(started)),
		}},
	}
}

func (h Handler) retryableResult(claim sdk.Claim, started time.Time, class string) sdk.Result {
	result := h.terminalResult(claim, started, class)
	result.State = sdk.ResultRetryable
	return result
}

func latencyMillis(d time.Duration) int {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > 1<<31-1 {
		return 1<<31 - 1
	}
	return int(ms)
}
