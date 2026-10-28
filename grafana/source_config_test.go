// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"errors"
	"testing"
)

func TestDefaultClientFactoryBuildsHTTPClient(t *testing.T) {
	t.Parallel()

	client, err := defaultClientFactory(TargetConfig{
		BaseURL: "https://grafana.example.test",
		Token:   "test-token",
	})
	if err != nil {
		t.Fatalf("defaultClientFactory(valid) error = %v, want nil", err)
	}
	httpClient, ok := client.(HTTPClient)
	if !ok {
		t.Fatalf("defaultClientFactory() type = %T, want grafana.HTTPClient", client)
	}
	if httpClient.baseURL.Host != "grafana.example.test" {
		t.Errorf("baseURL host = %q, want grafana.example.test", httpClient.baseURL.Host)
	}
}

func TestDefaultClientFactoryRejectsBadTarget(t *testing.T) {
	t.Parallel()

	if _, err := defaultClientFactory(TargetConfig{BaseURL: "://bad-url", Token: "test-token"}); err == nil {
		t.Error("defaultClientFactory(bad URL) = nil, want a parse error")
	}
	if _, err := defaultClientFactory(TargetConfig{BaseURL: "https://grafana.example.test"}); err == nil {
		t.Error("defaultClientFactory(empty token) = nil, want a token error")
	}
}

func TestNewClaimedSourceUsesDefaultFactory(t *testing.T) {
	t.Parallel()

	source, err := NewClaimedSource(SourceConfig{
		CollectorInstanceID: "collector-grafana",
		Targets: []TargetConfig{{
			ScopeID:    "grafana:prod",
			InstanceID: "prod",
			BaseURL:    "https://grafana.example.test",
			Token:      "test-token",
			Enabled:    true,
		}},
	})
	if err != nil {
		t.Fatalf("NewClaimedSource(default factory) error = %v, want nil", err)
	}
	if _, ok := source.targets["grafana:prod"]; !ok {
		t.Error("default-factory source has no grafana:prod target, want it registered")
	}
}

func TestNewClaimedSourceRejectsBadConfig(t *testing.T) {
	t.Parallel()

	validTarget := TargetConfig{
		ScopeID:    "grafana:prod",
		InstanceID: "prod",
		BaseURL:    "https://grafana.example.test",
		Token:      "test-token",
		Enabled:    true,
	}
	factory := func(TargetConfig) (EvidenceClient, error) { return stubClient{}, nil }
	failingFactory := func(TargetConfig) (EvidenceClient, error) {
		return nil, errors.New("client boom")
	}

	tests := []struct {
		name   string
		config SourceConfig
	}{
		{"empty instance ID", SourceConfig{Targets: []TargetConfig{validTarget}, ClientFactory: factory}},
		{"unsupported provider", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", InstanceID: "i", BaseURL: "https://g.test", Token: "t", Provider: "prometheus", Enabled: true}},
			ClientFactory:       factory,
		}},
		{"missing scope", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{InstanceID: "i", BaseURL: "https://g.test", Token: "t", Enabled: true}},
			ClientFactory:       factory,
		}},
		{"missing instance", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", BaseURL: "https://g.test", Token: "t", Enabled: true}},
			ClientFactory:       factory,
		}},
		{"missing base URL", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", InstanceID: "i", Token: "t", Enabled: true}},
			ClientFactory:       factory,
		}},
		{"missing token", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", InstanceID: "i", BaseURL: "https://g.test", Enabled: true}},
			ClientFactory:       factory,
		}},
		{"limit over max", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", InstanceID: "i", BaseURL: "https://g.test", Token: "t", ResourceLimit: maxResourceLimit + 1, Enabled: true}},
			ClientFactory:       factory,
		}},
		{"duplicate scope", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{validTarget, validTarget},
			ClientFactory:       factory,
		}},
		{"factory error", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{validTarget},
			ClientFactory:       failingFactory,
		}},
		{"no enabled targets", SourceConfig{
			CollectorInstanceID: "collector-grafana",
			Targets:             []TargetConfig{{ScopeID: "x", InstanceID: "i", BaseURL: "https://g.test", Token: "t"}},
			ClientFactory:       factory,
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewClaimedSource(tt.config); err == nil {
				t.Errorf("NewClaimedSource(%s) = nil, want an error", tt.name)
			}
		})
	}
}

func TestSchemaVersionRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	if version, ok := SchemaVersionForKind("observability.declared_dashboard"); ok || version != "" {
		t.Errorf("SchemaVersionForKind(declared) = %q, %v; want \"\", false", version, ok)
	}
	if err := CheckSchemaVersion("observability.declared_dashboard", SchemaVersionV1); err == nil {
		t.Error("CheckSchemaVersion(declared, 1.0.0) = nil, want rejection")
	}
}
