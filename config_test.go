// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"strings"
	"testing"
	"time"

	"github.com/eshu-hq/eshu-collector-grafana/grafana"
)

func testEnv(key string) string {
	switch key {
	case "GRAFANA_TOKEN":
		return "test-token"
	case CollectorInstanceIDEnv:
		return "collector-grafana"
	}
	return ""
}

// TestParseSettingsResolvesTargets proves the activation config maps onto
// the claim-driven source: tokens arrive by env reference (never inline),
// disabled targets are dropped before client construction, and the claimed
// scope must be configured.
func TestParseSettingsResolvesTargets(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(testConfig(), "grafana:prod", testEnv)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v, want nil", err)
	}
	if len(settings.Targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(settings.Targets))
	}
	target := settings.Targets[0]
	if target.ScopeID != "grafana:prod" || target.InstanceID != "prod" {
		t.Fatalf("target = %+v, want the configured scope and instance", target)
	}
	if target.Token != "test-token" {
		t.Fatalf("token was not resolved from the environment")
	}
	if target.BaseURL != "https://grafana.example.test" {
		t.Fatalf("base_url = %q, want the configured URL", target.BaseURL)
	}
}

// TestParseSettingsRejectsBadConfig proves misconfiguration fails closed:
// unknown keys (a misspelled limit must never be silently ignored), no
// targets, no enabled targets, duplicate scopes, and a claim scope with no
// configured target.
func TestParseSettingsRejectsBadConfig(t *testing.T) {
	t.Parallel()

	duplicate := map[string]any{"targets": []any{
		map[string]any{"scope_id": "grafana:prod", "instance_id": "a", "base_url": "https://a.example.test", "token_env": "GRAFANA_TOKEN"},
		map[string]any{"scope_id": "grafana:prod", "instance_id": "b", "base_url": "https://b.example.test", "token_env": "GRAFANA_TOKEN"},
	}}
	disabled := map[string]any{"targets": []any{
		map[string]any{"scope_id": "grafana:prod", "instance_id": "a", "base_url": "https://a.example.test", "token_env": "GRAFANA_TOKEN", "enabled": false},
	}}
	unknownKey := map[string]any{"targets": []any{
		map[string]any{"scope_id": "grafana:prod", "instance_id": "a", "base_url": "https://a.example.test", "token_env": "GRAFANA_TOKEN", "resource_limt": 5},
	}}

	cases := []struct {
		name   string
		config map[string]any
		scope  string
	}{
		{"no targets", map[string]any{}, "grafana:prod"},
		{"empty targets", map[string]any{"targets": []any{}}, "grafana:prod"},
		{"unconfigured scope", testConfig(), "grafana:other"},
		{"duplicate scope", duplicate, "grafana:prod"},
		{"disabled only", disabled, "grafana:prod"},
		{"unknown key", unknownKey, "grafana:prod"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseSettings(tc.config, tc.scope, testEnv); err == nil {
				t.Fatalf("ParseSettings(%s) = nil, want an error", tc.name)
			}
		})
	}
}

// TestParseSettingsRejectsUnserializableConfig proves a config that cannot
// survive a JSON round trip fails closed instead of silently degrading.
func TestParseSettingsRejectsUnserializableConfig(t *testing.T) {
	t.Parallel()

	raw := map[string]any{"targets": testConfig()["targets"], "fn": func() {}}
	if _, err := ParseSettings(raw, "grafana:prod", testEnv); err == nil {
		t.Fatal("ParseSettings(func value) = nil, want an error")
	}
}

// TestParseSettingsResolvesDeclaredUIDs proves declared UIDs map into the
// source target set while blank entries are dropped.
func TestParseSettingsResolvesDeclaredUIDs(t *testing.T) {
	t.Parallel()

	config := testConfig()
	config["targets"].([]any)[0].(map[string]any)["declared_uids"] = []string{"dash-1", "  ", "ds-1"}
	config["targets"].([]any)[0].(map[string]any)["stale_after"] = "30m"
	settings, err := ParseSettings(config, "grafana:prod", testEnv)
	if err != nil {
		t.Fatalf("ParseSettings() error = %v, want nil", err)
	}
	if len(settings.Targets[0].DeclaredUIDs) != 2 {
		t.Fatalf("declared_uids = %v, want the two trimmed UIDs", settings.Targets[0].DeclaredUIDs)
	}
	if settings.Targets[0].StaleAfter != 30*time.Minute {
		t.Fatalf("stale_after = %v, want 30m", settings.Targets[0].StaleAfter)
	}
}

// TestParseSettingsRejectsBadDurations proves a malformed stale_after fails
// closed instead of silently disabling the freshness bound.
func TestParseSettingsRejectsBadDurations(t *testing.T) {
	t.Parallel()

	config := testConfig()
	config["targets"].([]any)[0].(map[string]any)["stale_after"] = "not-a-duration"
	if _, err := ParseSettings(config, "grafana:prod", testEnv); err == nil {
		t.Fatal("ParseSettings(bad stale_after) = nil, want an error")
	}
}

// TestParseSettingsAcceptsNilGetenv proves a nil environment lookup resolves
// to empty values; token validation stays at source construction.
func TestParseSettingsAcceptsNilGetenv(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(testConfig(), "grafana:prod", nil)
	if err != nil {
		t.Fatalf("ParseSettings(nil getenv) error = %v, want nil", err)
	}
	if settings.Targets[0].Token != "" {
		t.Fatalf("token = %q, want empty with a nil lookup", settings.Targets[0].Token)
	}
}

// TestParseSettingsMissingTokenEnvFailsAtSource proves a token_env naming
// an unset variable resolves to an empty token here and fails closed at
// source construction (the source requires a token), never with an
// unauthenticated provider call.
func TestParseSettingsMissingTokenEnvFailsAtSource(t *testing.T) {
	t.Parallel()

	settings, err := ParseSettings(testConfig(), "grafana:prod", func(string) string { return "" })
	if err != nil {
		t.Fatalf("ParseSettings() error = %v, want nil (the source owns token validation)", err)
	}
	if settings.Targets[0].Token != "" {
		t.Fatalf("token = %q, want empty for an unset variable", settings.Targets[0].Token)
	}
	_, err = grafana.NewClaimedSource(grafana.SourceConfig{
		CollectorInstanceID: "collector-grafana",
		Targets:             settings.Targets,
	})
	if err == nil || !strings.Contains(err.Error(), "token is required") {
		t.Fatalf("NewClaimedSource(empty token) = %v, want the token-required failure", err)
	}
}
