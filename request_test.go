// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"

	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

func testClaim() sdk.Claim {
	return sdk.Claim{
		ComponentID:   ComponentID,
		InstanceID:    "collector-grafana",
		CollectorKind: CollectorKind,
		SourceSystem:  CollectorKind,
		Scope:         sdk.Scope{ID: "grafana:prod", Kind: "grafana_instance"},
		SourceRunID:   "run-1",
		GenerationID:  "grafana:generation-9",
		WorkItemID:    "work-1",
		FencingToken:  "7",
		Attempt:       1,
		Deadline:      time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		ConfigHandle:  "config-1",
	}
}

func testGrant() grant.Grant {
	now := time.Now().UTC()
	return grant.Grant{
		ProducerID: grant.ProducerID,
		Version:    grant.Version,
		Scope:      grant.Scope,
		Kinds: []grant.KindGrant{
			{Kind: "observability.source_instance", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.observed_dashboard", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.observed_rule", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.coverage_warning", SchemaVersions: []string{"1.0.0"}},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(720 * time.Hour),
	}
}

func testConfig() map[string]any {
	return map[string]any{"targets": []any{map[string]any{
		"provider":    "grafana",
		"scope_id":    "grafana:prod",
		"instance_id": "prod",
		"base_url":    "https://grafana.example.test",
		"token_env":   "GRAFANA_TOKEN",
		"enabled":     true,
	}}}
}

func encodeRequest(t *testing.T, claim sdk.Claim, config map[string]any, g grant.Grant) string {
	t.Helper()
	raw, err := json.Marshal(Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           claim,
		Contract:        Contract(),
		Config:          config,
		Grant:           g,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(raw)
}

// TestParseRequestAcceptsAFullHostRequest proves the wire shape the
// extension host writes: one bounded JSON document carrying claim, config,
// and the host-delivered producer grant together.
func TestParseRequestAcceptsAFullHostRequest(t *testing.T) {
	t.Parallel()

	request, err := ParseRequest(strings.NewReader(encodeRequest(t, testClaim(), testConfig(), testGrant())))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v, want nil", err)
	}
	if request.Claim.GenerationID != "grafana:generation-9" {
		t.Fatalf("claim generation = %q, want the request claim", request.Claim.GenerationID)
	}
	if len(request.Grant.Kinds) != 4 {
		t.Fatalf("grant kinds = %d, want the four delivered kinds", len(request.Grant.Kinds))
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TestParseRequestSurfacesReadErrors proves a stdin failure fails the parse
// instead of decoding a truncated document.
func TestParseRequestSurfacesReadErrors(t *testing.T) {
	t.Parallel()

	if _, err := ParseRequest(errReader{err: errors.New("stdin boom")}); err == nil {
		t.Fatal("ParseRequest(read error) = nil, want an error")
	}
}

// TestParseRequestRejectsBadInput proves malformed host input fails before
// any provider contact: unknown fields, trailing JSON, oversize documents,
// wrong protocol, and claims missing identity or naming another collector
// kind never reach the source.
func TestParseRequestRejectsBadInput(t *testing.T) {
	t.Parallel()

	valid := encodeRequest(t, testClaim(), testConfig(), testGrant())
	oversize := `{"protocol_version":"collector-sdk/v1alpha1",` + strings.Repeat(`"pad":"x",`, 1<<20)
	wrongKind := testClaim()
	wrongKind.CollectorKind = "prometheus_mimir"
	blankScope := testClaim()
	blankScope.Scope.ID = ""

	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"not JSON", "{"},
		{"unknown field", strings.Replace(valid, `"claim":`, `"claimz":1,"claim":`, 1)},
		{"trailing JSON", valid + " {}"},
		{"oversize", oversize},
		{"wrong protocol", strings.Replace(valid, "collector-sdk/v1alpha1", "collector-sdk/v9", 1)},
		{"wrong collector kind", encodeRequest(t, wrongKind, testConfig(), testGrant())},
		{"blank scope", encodeRequest(t, blankScope, testConfig(), testGrant())},
		{"syntax error", `{"protocol_version": }`},
		{"type error", strings.Replace(valid, `"protocol_version":"collector-sdk/v1alpha1"`, `"protocol_version":123`, 1)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseRequest(strings.NewReader(tc.input)); err == nil {
				t.Fatalf("ParseRequest(%q) = nil, want an error", tc.name)
			}
		})
	}
}
