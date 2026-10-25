// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector"
)

// Ported from go/internal/collector/observability/grafana/envelope_test.go
// in eshu-hq/eshu. Type mapping: facts.Envelope -> collector.Fact
// (FactKind -> Kind, CollectorKind -> SourceRef.SourceSystem,
// SourceRef.SourceURI -> SourceRef.URI); facts kind/schema/confidence
// constants -> this package's Kind*/SchemaVersionV1/reportedConfidence.

func TestObservedResourceEnvelopeRedactsSensitiveGrafanaFields(t *testing.T) {
	t.Parallel()

	ctx := testEnvelopeContext()
	dashboard := Resource{
		Class:           ResourceClassDashboard,
		UID:             "dash-checkout",
		Title:           "Checkout Latency",
		FolderUID:       "folder-prod",
		URL:             "https://grafana.example.internal/d/dash-checkout?token=secret",
		UpdatedAt:       time.Date(2026, 6, 1, 12, 5, 0, 0, time.UTC),
		ManuallyCreated: true,
		DriftReason:     DriftReasonManualProviderResource,
	}

	env, err := NewObservedResourceEnvelope(ctx, dashboard)
	if err != nil {
		t.Fatalf("NewObservedResourceEnvelope() error = %v, want nil", err)
	}

	assertObservabilityEnvelope(t, env, KindObservedDashboard)
	assertPayload(t, env, "source_class", SourceClassObserved)
	assertPayload(t, env, "source_kind", SourceKindGrafana)
	assertPayload(t, env, "resource_class", ResourceClassDashboard)
	assertPayload(t, env, "provider_object_uid", "dash-checkout")
	assertPayload(t, env, "folder_uid", "folder-prod")
	assertPayload(t, env, "title_present", true)
	assertPayload(t, env, "url_present", true)
	assertPayload(t, env, "url_redacted", true)
	assertPayload(t, env, "manually_created", true)
	assertPayload(t, env, "drift_candidate_reason", DriftReasonManualProviderResource)
	assertPayload(t, env, "redaction_state", "applied")
	assertPayload(t, env, "freshness_state", FreshnessCurrent)
	assertPayload(t, env, "outcome", OutcomeObserved)
	assertPayloadKeysAbsent(t, env, "url")
	assertPayloadForbidden(t, env, "Checkout Latency", "grafana.example.internal", "token=secret")
	if got := fmt.Sprint(env.Payload["title_fingerprint"]); !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("title_fingerprint = %q, want sha256 fingerprint", got)
	}
	if got := fmt.Sprint(env.Payload["url_fingerprint"]); !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("url_fingerprint = %q, want sha256 fingerprint", got)
	}
	if !strings.Contains(env.StableKey, ctx.GenerationID) {
		t.Fatalf("StableKey = %q, want generation-scoped key", env.StableKey)
	}
}

func TestObservedRuleEnvelopeDropsQueryModelAndContactFields(t *testing.T) {
	t.Parallel()

	ctx := testEnvelopeContext()
	rule := AlertRule{
		UID:             "rule-checkout-latency",
		Title:           "Checkout Latency",
		RuleGroup:       "checkout.rules",
		FolderUID:       "folder-prod",
		DatasourceUID:   "prometheus-prod",
		Condition:       "B",
		Model:           map[string]any{"expr": "histogram_quantile(0.95, rate(secret_bucket[5m]))"},
		ContactPoint:    "oncall@example.com",
		NotificationURL: "https://hooks.example.internal/secret",
	}

	env, err := NewObservedRuleEnvelope(ctx, rule)
	if err != nil {
		t.Fatalf("NewObservedRuleEnvelope() error = %v, want nil", err)
	}

	assertObservabilityEnvelope(t, env, KindObservedRule)
	assertPayload(t, env, "alert_rule_uid", "rule-checkout-latency")
	assertPayload(t, env, "rule_group", "checkout.rules")
	assertPayload(t, env, "folder_uid", "folder-prod")
	assertPayload(t, env, "datasource_uid", "prometheus-prod")
	assertPayload(t, env, "query_model_redacted", true)
	assertPayload(t, env, "contact_point_redacted", true)
	assertPayload(t, env, "notification_url_redacted", true)
	assertPayloadKeysAbsent(t, env, "model", "contact_point", "notification_url")
	assertPayloadForbidden(t, env, "Checkout Latency", "histogram_quantile", "secret_bucket", "oncall@example.com", "hooks.example.internal")
}

func TestObservedRuleEnvelopePreservesStaleState(t *testing.T) {
	t.Parallel()

	ctx := testEnvelopeContext()
	rule := AlertRule{
		UID:            "rule-stale",
		Title:          "Stale Alert",
		RuleGroup:      "checkout.rules",
		FreshnessState: FreshnessStale,
		Outcome:        OutcomeStale,
	}

	env, err := NewObservedRuleEnvelope(ctx, rule)
	if err != nil {
		t.Fatalf("NewObservedRuleEnvelope() error = %v, want nil", err)
	}

	assertObservabilityEnvelope(t, env, KindObservedRule)
	assertPayload(t, env, "freshness_state", FreshnessStale)
	assertPayload(t, env, "outcome", OutcomeStale)
}

func TestCoverageWarningEnvelopeUsesBoundedIdentity(t *testing.T) {
	t.Parallel()

	ctx := testEnvelopeContext()
	warning := Warning{
		ResourceClass: ResourceClassAlertRule,
		ResourceID:    "rule-checkout-latency",
		Reason:        WarningPermissionHidden,
	}

	env, err := NewCoverageWarningEnvelope(ctx, warning)
	if err != nil {
		t.Fatalf("NewCoverageWarningEnvelope() error = %v, want nil", err)
	}

	assertObservabilityEnvelope(t, env, KindCoverageWarning)
	assertPayload(t, env, "warning_kind", WarningPermissionHidden)
	assertPayload(t, env, "resource_class", ResourceClassAlertRule)
	assertPayload(t, env, "provider_object_uid", "rule-checkout-latency")
	assertPayload(t, env, "outcome", OutcomePermissionHidden)
	assertPayload(t, env, "redaction_state", "none")
}

func TestCoverageWarningEnvelopeUsesTruncatedReason(t *testing.T) {
	t.Parallel()

	if WarningTruncated != "truncated" {
		t.Fatalf("WarningTruncated = %q, want %q", WarningTruncated, "truncated")
	}

	ctx := testEnvelopeContext()
	warning := Warning{
		ResourceClass: ResourceClassDatasource,
		Reason:        WarningTruncated,
	}

	env, err := NewCoverageWarningEnvelope(ctx, warning)
	if err != nil {
		t.Fatalf("NewCoverageWarningEnvelope() error = %v, want nil", err)
	}

	assertObservabilityEnvelope(t, env, KindCoverageWarning)
	assertPayload(t, env, "warning_kind", WarningTruncated)
	assertPayload(t, env, "resource_class", ResourceClassDatasource)
}

func testEnvelopeContext() EnvelopeContext {
	return EnvelopeContext{
		ScopeID:             "grafana:instance:prod",
		GenerationID:        "generation-2026-06-01T12:00:00Z",
		CollectorInstanceID: "collector-grafana-prod",
		FencingToken:        42,
		ObservedAt:          time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		SourceURI:           "https://grafana.example.internal",
		SourceInstanceID:    "grafana-prod",
	}
}

func assertObservabilityEnvelope(t *testing.T, env collector.Fact, kind string) {
	t.Helper()
	if env.Kind != kind {
		t.Fatalf("Kind = %q, want %q", env.Kind, kind)
	}
	if env.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("SchemaVersion = %q, want %q", env.SchemaVersion, SchemaVersionV1)
	}
	if env.SourceRef.SourceSystem != CollectorKind {
		t.Fatalf("SourceSystem = %q, want %q", env.SourceRef.SourceSystem, CollectorKind)
	}
	if env.SourceConfidence != reportedConfidence {
		t.Fatalf("SourceConfidence = %q, want %q", env.SourceConfidence, reportedConfidence)
	}
	// Divergence from in-tree, required by the published SDK: core
	// facts.Ref carries no URI, but collector.Fact.SourceRef.URI is a
	// required SDK admission field (ValidateResult rejects an empty
	// source_ref.uri). The producer therefore emits the instance URI
	// and the SDK validator — not omission — is the credential guard:
	// no userinfo, no file scheme, no sensitive query values.
	assertSourceURISDKClean(t, env)
}

func assertSourceURISDKClean(t *testing.T, env collector.Fact) {
	t.Helper()
	raw := env.SourceRef.URI
	if strings.TrimSpace(raw) == "" {
		t.Fatalf("SourceRef.URI is empty, want the instance URI (SDK requires source_ref.uri)")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("SourceRef.URI = %q does not parse: %v", raw, err)
	}
	if parsed.User != nil {
		t.Fatalf("SourceRef.URI = %q carries userinfo", raw)
	}
	if parsed.Scheme == "file" || strings.HasPrefix(raw, "/") {
		t.Fatalf("SourceRef.URI = %q must not be a host-local path", raw)
	}
	lowered := strings.ToLower(raw)
	for _, forbidden := range []string{"token=", "secret", "password", "api_key", "apikey"} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("SourceRef.URI = %q carries sensitive value %q", raw, forbidden)
		}
	}
}

func assertPayload(t *testing.T, env collector.Fact, key string, want any) {
	t.Helper()
	if got := env.Payload[key]; got != want {
		t.Fatalf("%s payload[%s] = %#v, want %#v in %#v", env.Kind, key, got, want, env.Payload)
	}
}

func assertPayloadForbidden(t *testing.T, env collector.Fact, forbidden ...string) {
	t.Helper()
	rendered := strings.ToLower(fmt.Sprint(env.Payload))
	for _, value := range forbidden {
		if strings.Contains(rendered, strings.ToLower(value)) {
			t.Fatalf("%s payload leaks forbidden value %q: %#v", env.Kind, value, env.Payload)
		}
	}
}

func assertPayloadKeysAbsent(t *testing.T, env collector.Fact, forbidden ...string) {
	t.Helper()
	for _, key := range forbidden {
		if _, exists := env.Payload[key]; exists {
			t.Fatalf("%s payload has forbidden key %q: %#v", env.Kind, key, env.Payload)
		}
	}
}
