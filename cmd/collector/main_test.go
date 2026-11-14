// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/eshu-hq/eshu-sdk/collector"

	collector "github.com/eshu-hq/eshu-collector-grafana"
	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

// stubGrafana serves the three provider endpoints the entrypoint reads: one
// dashboard, one datasource, and one alert rule. It is the smallest provider
// surface that still yields source-instance, dashboard, and rule facts.
func stubGrafana(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/search":
			_, _ = w.Write([]byte(`[{"id":1,"uid":"dash-1","title":"Smoke","type":"dash-db","url":"/d/dash-1"}]`))
		case "/api/datasources":
			_, _ = w.Write([]byte(`[{"id":1,"uid":"ds-1","name":"Prom","type":"prometheus","url":"http://prom:9090"}]`))
		case "/api/v1/provisioning/alert-rules":
			_, _ = w.Write([]byte(`[{"uid":"rule-1","title":"High errors","ruleGroup":"smoke","folderUID":"general","condition":"A","updated":"2026-05-16T11:00:00Z"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func smokeClaim() sdk.Claim {
	return sdk.Claim{
		ComponentID:   collector.ComponentID,
		InstanceID:    "collector-grafana",
		CollectorKind: collector.CollectorKind,
		SourceSystem:  collector.CollectorKind,
		Scope:         sdk.Scope{ID: "grafana:prod", Kind: "grafana_instance"},
		SourceRunID:   "smoke-run",
		GenerationID:  "grafana:smoke:1",
		WorkItemID:    "smoke-work",
		FencingToken:  "7",
		Attempt:       1,
		Deadline:      time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		ConfigHandle:  "smoke-config",
	}
}

func smokeConfig() map[string]any {
	return map[string]any{"targets": []any{map[string]any{
		"provider":    "grafana",
		"scope_id":    "grafana:prod",
		"instance_id": "prod",
		"base_url":    "https://grafana.example.test",
		"token_env":   "GRAFANA_TOKEN",
		"enabled":     true,
	}}}
}

func smokeGrant() grant.Grant {
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

func smokeRequest(t *testing.T, serverURL string, g grant.Grant) []byte {
	t.Helper()
	base := smokeConfig()
	for _, item := range base["targets"].([]any) {
		item.(map[string]any)["base_url"] = serverURL
	}
	request := collector.Request{
		ProtocolVersion: sdk.ProtocolVersionV1Alpha1,
		Claim:           smokeClaim(),
		Contract:        collector.Contract(),
		Config:          base,
		Grant:           g,
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("json.Marshal(request) error = %v", err)
	}
	return raw
}

func testGetenv(key string) string {
	switch key {
	case "GRAFANA_TOKEN":
		return "test-token"
	case collector.CollectorInstanceIDEnv:
		return "collector-grafana"
	}
	return ""
}

func fixedNow() time.Time {
	return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
}

// TestRunClaimServesOneRequestAsOneResult proves the stdin/stdout contract
// end to end through the command: one Request in, one Result out with facts,
// and the claim echoed back for the host identity check. Both arg shapes
// (bare and --sdk-stdio) behave the same.
func TestRunClaimServesOneRequestAsOneResult(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{nil, {"--sdk-stdio"}} {
		server := stubGrafana(t)
		var stdout, stderr bytes.Buffer
		err := runClaim(context.Background(), args,
			bytes.NewReader(smokeRequest(t, server.URL, smokeGrant())),
			&stdout, &stderr, testGetenv, fixedNow)
		if err != nil {
			t.Fatalf("runClaim(%v) error = %v, stderr = %s", args, err, stderr.String())
		}
		var result sdk.Result
		decoder := json.NewDecoder(&stdout)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if result.State != sdk.ResultComplete && result.State != sdk.ResultPartial {
			t.Fatalf("result state = %q, want complete or partial", result.State)
		}
		if result.ProtocolVersion != sdk.ProtocolVersionV1Alpha1 {
			t.Fatalf("protocol version = %q, want %q", result.ProtocolVersion, sdk.ProtocolVersionV1Alpha1)
		}
		if result.Claim.GenerationID != "grafana:smoke:1" {
			t.Fatalf("claim echo generation = %q, want the request claim", result.Claim.GenerationID)
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
}

// TestRunClaimFailsClosedWithoutAGrant proves an unwired host delivers a
// terminal result with zero facts instead of unauthenticated evidence.
func TestRunClaimFailsClosedWithoutAGrant(t *testing.T) {
	t.Parallel()

	server := stubGrafana(t)
	var stdout, stderr bytes.Buffer
	err := runClaim(context.Background(), nil,
		bytes.NewReader(smokeRequest(t, server.URL, grant.Grant{})),
		&stdout, &stderr, testGetenv, fixedNow)
	if err != nil {
		t.Fatalf("runClaim() error = %v, want a terminal result, not a process failure", err)
	}
	var result sdk.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.State != sdk.ResultTerminal {
		t.Fatalf("result state = %q, want terminal without a grant", result.State)
	}
	if len(result.Facts) != 0 {
		t.Fatalf("result carries %d facts, want zero without a grant", len(result.Facts))
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("stdout boom") }

// TestRunClaimSurfacesWriteErrors proves a stdout failure fails the command
// instead of silently dropping the result the host is waiting for.
func TestRunClaimSurfacesWriteErrors(t *testing.T) {
	t.Parallel()

	server := stubGrafana(t)
	err := runClaim(context.Background(), nil,
		bytes.NewReader(smokeRequest(t, server.URL, smokeGrant())),
		errWriter{}, &bytes.Buffer{}, testGetenv, fixedNow)
	if err == nil {
		t.Fatal("runClaim(write error) = nil, want an error")
	}
}

// TestRunRejectsUnexpectedArguments proves the run wrapper carries the claim
// runner's argument rejection to the process exit.
func TestRunRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()

	err := run(context.Background(), []string{"extra"},
		strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{}, testGetenv)
	if err == nil {
		t.Fatal("run(extra arg) = nil, want an error")
	}
}

// TestRunClaimRejectsBadInput proves malformed host input fails the command
// before any provider contact: no partial result is written to stdout.
func TestRunClaimRejectsBadInput(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := runClaim(context.Background(), nil,
		strings.NewReader("{}"), &stdout, &bytes.Buffer{}, testGetenv, fixedNow)
	if err == nil {
		t.Fatal("runClaim({}) = nil, want an error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial result", stdout.String())
	}
}

// TestRunClaimRejectsUnexpectedArguments pins the CLI surface: the command
// serves one claim on stdio and takes no positional arguments.
func TestRunClaimRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()

	err := runClaim(context.Background(), []string{"extra"},
		strings.NewReader("{}"), &bytes.Buffer{}, &bytes.Buffer{}, testGetenv, fixedNow)
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") {
		t.Fatalf("runClaim(extra arg) error = %v, want the argument rejection", err)
	}
}
