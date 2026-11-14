// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"context"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"
	"github.com/eshu-hq/eshu-sdk/collector/transport"

	"github.com/eshu-hq/eshu-collector-grafana/grafana"
)

type stubEvidenceClient struct {
	result grafana.CollectionResult
	err    error
}

func (s stubEvidenceClient) CollectObservedMetadata(context.Context, grafana.TargetConfig) (grafana.CollectionResult, error) {
	return s.result, s.err
}

func handleRequest(claimant sdk.Claim, config map[string]any, client grafana.EvidenceClient) sdk.Result {
	handler := Handler{
		Getenv: testEnv,
		Now:    func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) },
		NewClient: func(grafana.TargetConfig) (grafana.EvidenceClient, error) {
			return client, nil
		},
	}
	return handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claimant,
		Contract:        Contract(),
		Config:          config,
		Grant:           testGrant(),
	})
}

func stubSnapshot() grafana.CollectionResult {
	return grafana.CollectionResult{
		Resources: []grafana.Resource{{
			Class: grafana.ResourceClassDashboard,
			ID:    42,
			UID:   "dashboard-uid-1",
			Title: "Checkout overview",
		}},
		Rules: []grafana.AlertRule{{
			UID:   "alert-rule-uid-1",
			Title: "High error rate",
		}},
		ObservedAt: time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC),
	}
}

// TestHandleServesOneClaimAsOneResult proves the entrypoint wiring: one
// host claim plus the delivered grant drives the claim-driven source to a
// complete result with facts, and the claim is echoed back for the host
// identity check.
func TestHandleServesOneClaimAsOneResult(t *testing.T) {
	t.Parallel()

	result := handleRequest(testClaim(), testConfig(), stubEvidenceClient{result: stubSnapshot()})
	if result.State != sdk.ResultComplete && result.State != sdk.ResultPartial {
		t.Fatalf("result state = %q, want complete or partial", result.State)
	}
	if result.ProtocolVersion != sdk.ProtocolVersionV1Alpha1 {
		t.Fatalf("protocol version = %q, want %q", result.ProtocolVersion, sdk.ProtocolVersionV1Alpha1)
	}
	if result.Claim.GenerationID != "grafana:generation-9" {
		t.Fatalf("claim echo generation = %q, want the request claim", result.Claim.GenerationID)
	}
	if len(result.Facts) == 0 {
		t.Fatal("result carries no facts, want emitted evidence")
	}
	byKind := map[string]int{}
	for _, fact := range result.Facts {
		byKind[fact.Kind]++
	}
	for _, kind := range []string{
		"observability.source_instance",
		"observability.observed_dashboard",
		"observability.observed_rule",
	} {
		if byKind[kind] == 0 {
			t.Fatalf("result facts = %v, want a %s fact", result.Facts, kind)
		}
	}
}

// TestHandleFailsClosedWithoutAGrant proves the unwired-host design: the
// host must deliver the core-issued producer grant with the request. A zero
// grant denies at the emission edge, so the claim ends terminal with zero
// facts — never with unauthenticated evidence.
func TestHandleFailsClosedWithoutAGrant(t *testing.T) {
	t.Parallel()

	handler := Handler{
		Getenv: testEnv,
		Now:    func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) },
		NewClient: func(grafana.TargetConfig) (grafana.EvidenceClient, error) {
			return stubEvidenceClient{result: stubSnapshot()}, nil
		},
	}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           testClaim(),
		Contract:        Contract(),
		Config:          testConfig(),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("result state = %q, want terminal for a missing grant", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("result carries %d facts, want zero without a grant", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != FailureGrantDenied {
		t.Fatalf("result statuses = %+v, want one grant-denied failure", result.Statuses)
	}
}

// TestHandleMapsClaimFailures proves claim-routing failures keep their
// retry contract: an unconfigured scope releases the claim for retry, while
// a foreign instance or an invalid config terminal-fails it.
func TestHandleMapsClaimFailures(t *testing.T) {
	t.Parallel()

	foreign := testClaim()
	foreign.InstanceID = "collector-other"
	if result := handleRequest(foreign, testConfig(), stubEvidenceClient{}); result.State != sdk.ResultTerminal {
		t.Fatalf("foreign instance state = %q, want terminal", result.State)
	}
	otherScope := testClaim()
	otherScope.Scope.ID = "grafana:unconfigured"
	if result := handleRequest(otherScope, testConfig(), stubEvidenceClient{}); result.State != sdk.ResultRetryable {
		t.Fatalf("unconfigured scope state = %q, want retryable", result.State)
	}
	broken := map[string]any{"targets": []any{}}
	if result := handleRequest(testClaim(), broken, stubEvidenceClient{}); result.State != sdk.ResultTerminal {
		t.Fatalf("invalid config state = %q, want terminal", result.State)
	}
}

// TestHandleFailsClosedWithoutAnInstanceBinding proves the instance binding
// comes from the host environment, never the claim: an unset binding fails
// the claim terminal instead of collecting under an unattributable identity.
func TestHandleFailsClosedWithoutAnInstanceBinding(t *testing.T) {
	t.Parallel()

	handler := Handler{
		Getenv: func(key string) string {
			if key == "GRAFANA_TOKEN" {
				return "test-token"
			}
			return ""
		},
		NewClient: func(grafana.TargetConfig) (grafana.EvidenceClient, error) {
			return stubEvidenceClient{result: stubSnapshot()}, nil
		},
	}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           testClaim(),
		Contract:        Contract(),
		Config:          testConfig(),
		Grant:           testGrant(),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("result state = %q, want terminal without an instance binding", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("result carries %d facts, want zero without an instance binding", len(result.Facts))
	}
}

// TestHandleRejectsForeignCollectorKinds proves a claim naming another
// collector kind terminal-fails at the entrypoint before any config work.
func TestHandleRejectsForeignCollectorKinds(t *testing.T) {
	t.Parallel()

	claim := testClaim()
	claim.CollectorKind = "prometheus_mimir"
	result := handleRequest(claim, testConfig(), stubEvidenceClient{})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("foreign kind state = %q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("foreign kind carries %d facts, want zero", len(result.Facts))
	}
}

// TestHandleReleasesBlankScopeClaims proves a claim without scope identity
// cannot be routed and releases for retry instead of collecting anywhere.
func TestHandleReleasesBlankScopeClaims(t *testing.T) {
	t.Parallel()

	claim := testClaim()
	claim.Scope.ID = ""
	result := handleRequest(claim, testConfig(), stubEvidenceClient{})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("blank scope state = %q, want retryable", result.State)
	}
}

// TestHandleFailsClosedOnSourceConstruction proves a config that passes
// parsing but fails source validation (here: an empty token) ends terminal
// before any provider contact.
func TestHandleFailsClosedOnSourceConstruction(t *testing.T) {
	t.Parallel()

	handler := Handler{
		Getenv: func(key string) string {
			if key == CollectorInstanceIDEnv {
				return "collector-grafana"
			}
			return ""
		},
		NewClient: func(grafana.TargetConfig) (grafana.EvidenceClient, error) {
			return stubEvidenceClient{result: stubSnapshot()}, nil
		},
	}
	result := handler.Handle(context.Background(), Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           testClaim(),
		Contract:        Contract(),
		Config:          testConfig(),
		Grant:           testGrant(),
	})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("result state = %q, want terminal for an empty token", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("result carries %d facts, want zero", len(result.Facts))
	}
}

// TestHandleUsesDefaults proves the zero-value Handler resolves the empty
// environment lookup and the real HTTP client constructor.
func TestHandleUsesDefaults(t *testing.T) {
	t.Parallel()

	handler := Handler{}
	if got := handler.getenv("anything"); got != "" {
		t.Fatalf("zero Handler getenv = %q, want empty", got)
	}
	client, err := handler.newClient(grafana.TargetConfig{BaseURL: "https://grafana.example.test", Token: "test-token"})
	if err != nil {
		t.Fatalf("zero Handler newClient error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("zero Handler newClient = nil, want the HTTP client")
	}
	if got := latencyMillis(-time.Second); got != 0 {
		t.Fatalf("latencyMillis(-1s) = %d, want 0", got)
	}
	if got := latencyMillis(3 * time.Second); got != 3000 {
		t.Fatalf("latencyMillis(3s) = %d, want 3000", got)
	}
	if got := latencyMillis(time.Duration(1 << 62)); got != 1<<31-1 {
		t.Fatalf("latencyMillis(huge) = %d, want saturation", got)
	}
}

// TestHandleMapsProviderFailure proves a provider outage releases the claim
// for retry with the classified failure class and zero facts.
func TestHandleMapsProviderFailure(t *testing.T) {
	t.Parallel()

	providerErr := transport.NewProviderFailure("grafana", transport.FailureRetryable, false, context.DeadlineExceeded)
	result := handleRequest(testClaim(), testConfig(), stubEvidenceClient{err: providerErr})
	if result.State != sdk.ResultRetryable {
		t.Fatalf("provider outage state = %q, want retryable", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("provider outage carries %d facts, want zero", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != string(transport.FailureRetryable) {
		t.Fatalf("result statuses = %+v, want one retryable failure", result.Statuses)
	}
}

// TestHandleTerminalFailsTerminalProviderFailures proves a provider failure
// the classifier marks terminal (for example auth-denied) ends terminal with
// the classified failure class and zero facts.
func TestHandleTerminalFailsTerminalProviderFailures(t *testing.T) {
	t.Parallel()

	terminal := transport.NewProviderFailure("grafana", transport.FailureAuthDenied, true, context.DeadlineExceeded)
	result := handleRequest(testClaim(), testConfig(), stubEvidenceClient{err: terminal})
	if result.State != sdk.ResultTerminal {
		t.Fatalf("terminal provider failure state = %q, want terminal", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("terminal provider failure carries %d facts, want zero", len(result.Facts))
	}
	if len(result.Statuses) != 1 || result.Statuses[0].FailureClass != string(transport.FailureAuthDenied) {
		t.Fatalf("result statuses = %+v, want one auth-denied failure", result.Statuses)
	}
}
