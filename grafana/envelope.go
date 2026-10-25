// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"fmt"
	"strings"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/factschema"
	"github.com/eshu-hq/eshu-sdk/factschema/identity"
	observabilityv1 "github.com/eshu-hq/eshu-sdk/factschema/observability/v1"
)

// emittedFact is the collector-sdk/v1alpha1 fact envelope this producer
// emits. The core host projects it onto durable storage at admission:
// FactID assignment, fencing enforcement, and generation commit are
// host-owned and have no emission-half counterpart.
type emittedFact = collector.Fact

// reportedConfidence marks facts returned by an external system or API.
const reportedConfidence = collector.SourceConfidenceReported

// NewSourceInstanceEnvelope converts one live Grafana target snapshot into a
// source-instance fact.
func NewSourceInstanceEnvelope(ctx EnvelopeContext, stats CollectionStats) (collector.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return collector.Fact{}, err
	}
	payload := basePayload(ctx, OutcomeObserved, FreshnessCurrent)
	payload["resource_count"] = stats.Resources
	payload["rule_count"] = stats.Rules
	payload["warning_count"] = stats.Warnings
	payload["pages_fetched"] = stats.PagesFetched
	payload["partial"] = stats.Partial
	payload["truncated"] = stats.Truncated
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilitySourceInstance(observabilityv1.SourceInstance{
			SourceInstanceID: ctx.SourceInstanceID,
			Provider:         stringPtr(ProviderGrafana),
			SourceKind:       stringPtr(SourceKindGrafana),
			SourceClass:      stringPtr(SourceClassObserved),
			Outcome:          stringPtr(OutcomeObserved),
			FreshnessState:   stringPtr(FreshnessCurrent),
		})
	}); err != nil {
		return collector.Fact{}, err
	}
	stableKey := stableFactKey(KindSourceInstance, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
	})
	return observabilityFact(ctx, KindSourceInstance, stableKey, payload, ctx.SourceInstanceID), nil
}

// NewObservedResourceEnvelope converts one bounded live Grafana resource into
// an observed-dashboard fact.
func NewObservedResourceEnvelope(ctx EnvelopeContext, resource Resource) (collector.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return collector.Fact{}, err
	}
	resourceClass := strings.TrimSpace(resource.Class)
	if resourceClass == "" {
		return collector.Fact{}, fmt.Errorf("grafana resource class must not be blank")
	}
	uid := firstNonBlank(resource.UID, resource.Name, fmt.Sprint(resource.ID))
	if uid == "" || uid == "0" {
		return collector.Fact{}, fmt.Errorf("grafana resource identity must not be blank")
	}
	payload := basePayload(ctx, normalizedOutcome(resource.Outcome), normalizedFreshness(resource.FreshnessState))
	payload["resource_class"] = resourceClass
	payload["provider_object_uid"] = uid
	if value := strings.TrimSpace(resource.FolderUID); value != "" {
		payload["folder_uid"] = value
	}
	if value := strings.TrimSpace(resource.DatasourceType); value != "" {
		payload["datasource_type"] = value
	}
	if value := strings.TrimSpace(resource.DeclaredMatchState); value != "" {
		payload["declared_match_state"] = value
	} else {
		payload["declared_match_state"] = MatchStateNotCompared
	}
	setFingerprint(payload, "title", resource.Title)
	setFingerprint(payload, "name", resource.Name)
	setURLRedaction(payload, resource.URL, resource.URLRedacted)
	setTimePayload(payload, "updated_at", resource.UpdatedAt)
	if resource.ManuallyCreated {
		payload["manually_created"] = true
	}
	if value := strings.TrimSpace(resource.DriftReason); value != "" {
		payload["drift_candidate_reason"] = value
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilityObservedDashboard(observabilityv1.ObservedDashboard{
			SourceInstanceID:     ctx.SourceInstanceID,
			ProviderObjectUID:    uid,
			FolderUID:            optionalStringPtr(resource.FolderUID),
			Provider:             stringPtr(ProviderGrafana),
			SourceKind:           stringPtr(SourceKindGrafana),
			SourceClass:          stringPtr(SourceClassObserved),
			ResourceClass:        stringPtr(resourceClass),
			Outcome:              stringPtr(normalizedOutcome(resource.Outcome)),
			FreshnessState:       stringPtr(normalizedFreshness(resource.FreshnessState)),
			DriftCandidateReason: optionalStringPtr(resource.DriftReason),
			DeclaredMatchState:   stringPtr(firstNonBlank(resource.DeclaredMatchState, MatchStateNotCompared)),
		})
	}); err != nil {
		return collector.Fact{}, err
	}
	stableKey := stableFactKey(KindObservedDashboard, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"resource_class":     resourceClass,
		"provider_uid":       uid,
	})
	return observabilityFact(ctx, KindObservedDashboard, stableKey, payload, uid), nil
}

// NewObservedRuleEnvelope converts one bounded live Grafana alert rule into an
// observed-rule fact.
func NewObservedRuleEnvelope(ctx EnvelopeContext, rule AlertRule) (collector.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return collector.Fact{}, err
	}
	uid := strings.TrimSpace(rule.UID)
	if uid == "" {
		return collector.Fact{}, fmt.Errorf("grafana alert rule uid must not be blank")
	}
	payload, err := factschema.EncodeObservabilityObservedRule(observabilityv1.ObservedRule{
		SourceInstanceID:   ctx.SourceInstanceID,
		AlertRuleUID:       stringPtr(uid),
		FolderUID:          optionalStringPtr(rule.FolderUID),
		DatasourceUID:      optionalStringPtr(rule.DatasourceUID),
		RuleGroup:          optionalStringPtr(rule.RuleGroup),
		Provider:           stringPtr(ProviderGrafana),
		SourceKind:         stringPtr(SourceKindGrafana),
		SourceClass:        stringPtr(SourceClassObserved),
		ResourceClass:      stringPtr(ResourceClassAlertRule),
		Outcome:            stringPtr(normalizedOutcome(rule.Outcome)),
		FreshnessState:     stringPtr(normalizedFreshness(rule.FreshnessState)),
		DeclaredMatchState: stringPtr(firstNonBlank(rule.DeclaredMatchState, MatchStateNotCompared)),
	})
	if err != nil {
		return collector.Fact{}, err
	}
	addEnvelopeContextPayload(payload, ctx)
	setFingerprint(payload, "title", rule.Title)
	setTimePayload(payload, "updated_at", rule.UpdatedAt)
	if len(rule.Model) > 0 || rule.QueryModelRedacted {
		payload["query_model_redacted"] = true
	}
	if strings.TrimSpace(rule.ContactPoint) != "" || rule.ContactPointRedacted {
		payload["contact_point_redacted"] = true
	}
	if strings.TrimSpace(rule.NotificationURL) != "" || rule.NotificationURLRedacted {
		payload["notification_url_redacted"] = true
	}
	setRedactionState(payload)
	stableKey := stableFactKey(KindObservedRule, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"alert_rule_uid":     uid,
	})
	return observabilityFact(ctx, KindObservedRule, stableKey, payload, uid), nil
}

// NewCoverageWarningEnvelope converts a source-local warning into an
// observability coverage-warning fact.
func NewCoverageWarningEnvelope(ctx EnvelopeContext, warning Warning) (collector.Fact, error) {
	if err := validateEnvelopeContext(ctx); err != nil {
		return collector.Fact{}, err
	}
	reason := strings.TrimSpace(warning.Reason)
	if reason == "" {
		reason = WarningPartial
	}
	resourceClass := strings.TrimSpace(warning.ResourceClass)
	if resourceClass == "" {
		resourceClass = "unknown"
	}
	resourceID := strings.TrimSpace(warning.ResourceID)
	payload := basePayload(ctx, warningOutcome(reason), warningFreshness(reason))
	payload["warning_kind"] = reason
	payload["resource_class"] = resourceClass
	if resourceID != "" {
		payload["provider_object_uid"] = resourceID
	}
	setRedactionState(payload)
	if err := mergeContractPayload(payload, func() (map[string]any, error) {
		return factschema.EncodeObservabilityCoverageWarning(observabilityv1.CoverageWarning{
			SourceInstanceID:  ctx.SourceInstanceID,
			ProviderObjectUID: optionalStringPtr(resourceID),
			Provider:          stringPtr(ProviderGrafana),
			SourceKind:        stringPtr(SourceKindGrafana),
			SourceClass:       stringPtr(SourceClassObserved),
			ResourceClass:     stringPtr(resourceClass),
			Outcome:           stringPtr(warningOutcome(reason)),
			FreshnessState:    stringPtr(warningFreshness(reason)),
			WarningKind:       stringPtr(reason),
		})
	}); err != nil {
		return collector.Fact{}, err
	}
	stableKey := stableFactKey(KindCoverageWarning, ctx.GenerationID, map[string]any{
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
		"resource_class":     resourceClass,
		"resource_id":        resourceID,
		"reason":             reason,
	})
	return observabilityFact(
		ctx,
		KindCoverageWarning,
		stableKey,
		payload,
		firstNonBlank(resourceID, reason),
	), nil
}

func basePayload(ctx EnvelopeContext, outcome string, freshness string) map[string]any {
	payload := map[string]any{
		"collector_instance_id": ctx.CollectorInstanceID,
		"provider":              ProviderGrafana,
		"source_class":          SourceClassObserved,
		"source_kind":           SourceKindGrafana,
		"source_instance_id":    ctx.SourceInstanceID,
		"scope_id":              ctx.ScopeID,
		"generation_id":         ctx.GenerationID,
		"observed_at":           normalizedObservedAt(ctx.ObservedAt).Format(time.RFC3339Nano),
		"freshness_state":       freshness,
		"redaction_version":     RedactionVersion,
		"outcome":               outcome,
	}
	payload["provenance"] = map[string]any{
		"provider":           ProviderGrafana,
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
	}
	return payload
}

func addEnvelopeContextPayload(payload map[string]any, ctx EnvelopeContext) {
	payload["collector_instance_id"] = ctx.CollectorInstanceID
	payload["scope_id"] = ctx.ScopeID
	payload["generation_id"] = ctx.GenerationID
	payload["observed_at"] = normalizedObservedAt(ctx.ObservedAt).Format(time.RFC3339Nano)
	payload["redaction_version"] = RedactionVersion
	payload["provenance"] = map[string]any{
		"provider":           ProviderGrafana,
		"source_instance_id": ctx.SourceInstanceID,
		"scope_id":           ctx.ScopeID,
	}
}

func mergeContractPayload(payload map[string]any, encode func() (map[string]any, error)) error {
	encoded, err := encode()
	if err != nil {
		return err
	}
	for key, value := range encoded {
		payload[key] = value
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func observabilityFact(
	ctx EnvelopeContext,
	kind string,
	stableKey string,
	payload map[string]any,
	recordID string,
) collector.Fact {
	return collector.Fact{
		Kind:             kind,
		SchemaVersion:    SchemaVersionV1,
		StableKey:        stableKey,
		SourceConfidence: reportedConfidence,
		ObservedAt:       normalizedObservedAt(ctx.ObservedAt),
		Payload:          payload,
		SourceRef: collector.SourceRef{
			SourceSystem: CollectorKind,
			ScopeID:      ctx.ScopeID,
			GenerationID: ctx.GenerationID,
			FactKey:      stableKey,
			URI:          strings.TrimSpace(ctx.SourceURI),
			RecordID:     strings.TrimSpace(recordID),
		},
	}
}

func validateEnvelopeContext(ctx EnvelopeContext) error {
	if strings.TrimSpace(ctx.ScopeID) == "" {
		return fmt.Errorf("grafana envelope scope_id must not be blank")
	}
	if strings.TrimSpace(ctx.GenerationID) == "" {
		return fmt.Errorf("grafana envelope generation_id must not be blank")
	}
	if strings.TrimSpace(ctx.CollectorInstanceID) == "" {
		return fmt.Errorf("grafana envelope collector_instance_id must not be blank")
	}
	if strings.TrimSpace(ctx.SourceInstanceID) == "" {
		return fmt.Errorf("grafana envelope source_instance_id must not be blank")
	}
	return nil
}

func stableFactKey(kind string, generationID string, identityMap map[string]any) string {
	return strings.Join([]string{kind, generationID, identity.StableID(kind, identityMap)}, ":")
}

func normalizedObservedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func setTimePayload(payload map[string]any, key string, value time.Time) {
	if !value.IsZero() {
		payload[key] = value.UTC().Format(time.RFC3339Nano)
	}
}

func warningOutcome(reason string) string {
	switch reason {
	case WarningPermissionHidden:
		return OutcomePermissionHidden
	case WarningUnsupported:
		return OutcomeUnsupported
	case WarningManualProviderResource:
		return OutcomeObserved
	case WarningStale:
		return OutcomeStale
	default:
		// WarningTruncated and unrecognized reasons both map to a
		// generic partial-read outcome.
		return OutcomePartial
	}
}

func warningFreshness(reason string) string {
	switch reason {
	case WarningPermissionHidden:
		return FreshnessPermissionHidden
	case WarningStale:
		return FreshnessStale
	default:
		return FreshnessUnknown
	}
}

func normalizedOutcome(value string) string {
	switch strings.TrimSpace(value) {
	case OutcomePartial:
		return OutcomePartial
	case OutcomePermissionHidden:
		return OutcomePermissionHidden
	case OutcomeUnsupported:
		return OutcomeUnsupported
	case OutcomeRejected:
		return OutcomeRejected
	case OutcomeStale:
		return OutcomeStale
	default:
		return OutcomeObserved
	}
}

func normalizedFreshness(value string) string {
	switch strings.TrimSpace(value) {
	case FreshnessUnknown:
		return FreshnessUnknown
	case FreshnessStale:
		return FreshnessStale
	case FreshnessPermissionHidden:
		return FreshnessPermissionHidden
	default:
		return FreshnessCurrent
	}
}
