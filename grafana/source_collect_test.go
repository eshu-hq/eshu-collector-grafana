// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/collector/transport"

	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

type stubClient struct {
	result CollectionResult
	err    error
}

func (s stubClient) CollectObservedMetadata(context.Context, TargetConfig) (CollectionResult, error) {
	return s.result, s.err
}

func testGrant() grant.Grant {
	now := time.Now().UTC()
	return grant.Grant{
		ProducerID: grant.ProducerID,
		Version:    grant.Version,
		Scope:      grant.Scope,
		Kinds: []grant.KindGrant{
			{Kind: KindSourceInstance, SchemaVersions: []string{SchemaVersionV1}},
			{Kind: KindObservedDashboard, SchemaVersions: []string{SchemaVersionV1}},
			{Kind: KindObservedRule, SchemaVersions: []string{SchemaVersionV1}},
			{Kind: KindCoverageWarning, SchemaVersions: []string{SchemaVersionV1}},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(720 * time.Hour),
	}
}

func testClaim() collector.Claim {
	return collector.Claim{
		ComponentID:   "dev.eshu.collector.grafana",
		InstanceID:    "collector-grafana",
		CollectorKind: CollectorKind,
		SourceSystem:  CollectorKind,
		Scope:         collector.Scope{ID: "grafana:prod", Kind: ScopeKindGrafanaInstance},
		SourceRunID:   "run-1",
		GenerationID:  "grafana:generation-9",
		WorkItemID:    "work-1",
		FencingToken:  "fencing-7",
		Attempt:       1,
		Deadline:      time.Now().UTC().Add(time.Hour),
		ConfigHandle:  "config-1",
	}
}

func testSource(t *testing.T, client EvidenceClient) *ClaimedSource {
	t.Helper()

	source, err := NewClaimedSource(SourceConfig{
		CollectorInstanceID: "collector-grafana",
		Targets: []TargetConfig{{
			ScopeID:    "grafana:prod",
			InstanceID: "prod",
			BaseURL:    "https://grafana.example.test",
			Token:      "test-token",
			Enabled:    true,
		}},
		ClientFactory: func(TargetConfig) (EvidenceClient, error) { return client, nil },
	})
	if err != nil {
		t.Fatalf("NewClaimedSource() error = %v, want nil", err)
	}
	return source
}

func stubSnapshot() CollectionResult {
	return CollectionResult{
		Resources: []Resource{{
			Class: ResourceClassDashboard,
			ID:    42,
			UID:   "dashboard-uid-1",
			Title: "Checkout overview",
		}},
		Rules: []AlertRule{{
			UID:   "alert-rule-uid-1",
			Title: "High error rate",
		}},
		ObservedAt: time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC),
	}
}

// grantContract mirrors the grant draft: the four observed kinds at schema
// 1.0.0 with reported confidence. The validator self-check below proves the
// producer's output satisfies the published SDK's own admission rules.
func grantContract() collector.Contract {
	declare := func(kind string) collector.FactDeclaration {
		return collector.FactDeclaration{
			Kind:             kind,
			SchemaVersions:   []string{SchemaVersionV1},
			SourceConfidence: []collector.SourceConfidence{collector.SourceConfidenceReported},
		}
	}
	return collector.Contract{
		ProtocolVersion: collector.ProtocolVersionV1Alpha1,
		Facts: []collector.FactDeclaration{
			declare(KindSourceInstance),
			declare(KindObservedDashboard),
			declare(KindObservedRule),
			declare(KindCoverageWarning),
		},
	}
}

func TestCollectEmitsValidatorCleanResult(t *testing.T) {
	t.Parallel()

	source := testSource(t, stubClient{result: stubSnapshot()})
	got, err := source.Collect(context.Background(), testClaim(), testGrant())
	if err != nil {
		t.Fatalf("Collect() error = %v, want nil", err)
	}
	if got.State != collector.ResultComplete {
		t.Errorf("State = %q, want complete", got.State)
	}
	// Source instance + one resource + one rule.
	if len(got.Facts) != 3 {
		t.Fatalf("len(Facts) = %d, want 3", len(got.Facts))
	}
	if got.Generation.ID != "grafana:generation-9" {
		t.Errorf("Generation.ID = %q, want claim generation", got.Generation.ID)
	}
	for _, fact := range got.Facts {
		if fact.SourceRef.ScopeID != "grafana:prod" {
			t.Errorf("fact %q scope = %q, want grafana:prod", fact.Kind, fact.SourceRef.ScopeID)
		}
		if fact.SourceRef.FactKey != fact.StableKey {
			t.Errorf("fact %q source_ref.fact_key does not match stable_key", fact.Kind)
		}
	}
	report, err := collector.NewValidator(grantContract()).ValidateResult(got)
	if err != nil {
		t.Fatalf("ValidateResult() error = %v, want validator-clean output", err)
	}
	if report.FactCount != 3 {
		t.Errorf("report.FactCount = %d, want 3", report.FactCount)
	}
}

func TestCollectPartialReadStaysPartial(t *testing.T) {
	t.Parallel()

	snapshot := stubSnapshot()
	snapshot.Stats.Partial = true
	snapshot.Warnings = []Warning{{ResourceClass: ResourceClassDashboard, Reason: WarningRateLimited}}
	source := testSource(t, stubClient{result: snapshot})
	got, err := source.Collect(context.Background(), testClaim(), testGrant())
	if err != nil {
		t.Fatalf("Collect() error = %v, want nil", err)
	}
	if got.State != collector.ResultPartial {
		t.Fatalf("State = %q, want partial", got.State)
	}
	if _, err := collector.NewValidator(grantContract()).ValidateResult(got); err != nil {
		t.Fatalf("ValidateResult(partial) error = %v, want validator-clean output", err)
	}
}

func TestCollectFailsClosedWithoutGrant(t *testing.T) {
	t.Parallel()

	source := testSource(t, stubClient{result: stubSnapshot()})
	if _, err := source.Collect(context.Background(), testClaim(), grant.Grant{}); err == nil {
		t.Error("Collect(zero grant) = nil, want fail-closed grant denial")
	} else if !errors.Is(err, grant.ErrDenied) {
		t.Errorf("Collect(zero grant) error = %v, want grant.ErrDenied", err)
	}
}

func TestCollectRejectsForeignClaim(t *testing.T) {
	t.Parallel()

	source := testSource(t, stubClient{result: stubSnapshot()})

	wrongInstance := testClaim()
	wrongInstance.InstanceID = "collector-other"
	if _, err := source.Collect(context.Background(), wrongInstance, testGrant()); err == nil {
		t.Error("Collect(foreign instance) = nil, want rejection")
	}

	wrongKind := testClaim()
	wrongKind.CollectorKind = "prometheus"
	if _, err := source.Collect(context.Background(), wrongKind, testGrant()); err == nil {
		t.Error("Collect(foreign kind) = nil, want rejection")
	}

	blankGeneration := testClaim()
	blankGeneration.GenerationID = ""
	if _, err := source.Collect(context.Background(), blankGeneration, testGrant()); err == nil {
		t.Error("Collect(blank generation) = nil, want rejection")
	}
}

func TestCollectUnknownScopeIsRetryable(t *testing.T) {
	t.Parallel()

	source := testSource(t, stubClient{result: stubSnapshot()})
	claim := testClaim()
	claim.Scope.ID = "grafana:unknown"
	_, err := source.Collect(context.Background(), claim, testGrant())
	if err == nil {
		t.Fatal("Collect(unknown scope) = nil, want retryable failure")
	}
	var failure transport.ProviderFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Collect(unknown scope) error type = %T, want transport.ProviderFailure", err)
	}
	if failure.FailureClass() != FailureRetryable {
		t.Errorf("FailureClass() = %q, want retryable", failure.FailureClass())
	}
}

func TestCollectClassifiesProviderFailure(t *testing.T) {
	t.Parallel()

	providerErr := GrafanaError{Provider: "grafana", StatusCode: 429, Message: "Too Many Requests"}
	source := testSource(t, stubClient{err: providerErr})
	_, err := source.Collect(context.Background(), testClaim(), testGrant())
	if err == nil {
		t.Fatal("Collect(provider 429) = nil, want classified failure")
	}
	var failure transport.ProviderFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Collect(provider 429) error type = %T, want transport.ProviderFailure", err)
	}
	if failure.FailureClass() != FailureRateLimited {
		t.Errorf("FailureClass() = %q, want rate_limited", failure.FailureClass())
	}
}
