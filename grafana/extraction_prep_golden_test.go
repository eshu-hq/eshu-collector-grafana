// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"encoding/json"
	"testing"
)

// Golden vectors cut from the in-tree constructors
// (go/internal/collector/observability/grafana in eshu-hq/eshu,
// extractionPrepContext/Resource/Rule + Warning{Dashboard, RateLimited}).
// These pin the extracted producer to byte-identical emission behavior:
// any change to stable keys, title fingerprinting, or payload bytes
// breaks this test. Regenerate only from the in-tree constructors, never
// from this package's own output.
const (
	goldenStableKeySourceInstance = "observability.source_instance:grafana:generation-9:4c1e93a919e505519d55701e88978920c726c505e4a6ba515d074d80a0b723a9"
	goldenStableKeyDashboard      = "observability.observed_dashboard:grafana:generation-9:4660a8f348b294dd01005ff4ba59feb9a555ede54b433d7ccebff2f77a59132c"
	goldenStableKeyRule           = "observability.observed_rule:grafana:generation-9:eee635bc6f183a11e0181d37c4fb9ca9cf6ef10139abb881e321c7b0b546b745"
	goldenStableKeyWarning        = "observability.coverage_warning:grafana:generation-9:e24820af56d17a15a091416c55fdcebfc3194abf00e4c8ab9f59a0503cb7c196"

	goldenTitleFingerprint = "sha256:41b87cc3c00fad40685e1cb80f6e9ccb628c1d2fbce0d74e6c4879ed9785debe"
)

const goldenDashboardPayload = `{"collector_instance_id":"collector-grafana","datasource_type":"prometheus","declared_match_state":"not_compared","folder_uid":"folder-uid-1","freshness_state":"current","generation_id":"grafana:generation-9","observed_at":"2026-05-16T11:00:00Z","outcome":"observed","provenance":{"provider":"grafana","scope_id":"grafana:prod","source_instance_id":"prod"},"provider":"grafana","provider_object_uid":"dashboard-uid-1","redaction_state":"applied","redaction_version":"observability-live-grafana-v1","resource_class":"dashboard","scope_id":"grafana:prod","source_class":"observed","source_instance_id":"prod","source_kind":"grafana","title_fingerprint":"sha256:41b87cc3c00fad40685e1cb80f6e9ccb628c1d2fbce0d74e6c4879ed9785debe","title_present":true,"updated_at":"2026-05-16T10:00:00Z","url_fingerprint":"sha256:764fc085391fc7e6282970d15de0d06ad73a69a305845d3215ffa2dd3c5c68f3","url_present":true,"url_redacted":true}`

func TestExtractionPrepGoldenVectors(t *testing.T) {
	t.Parallel()

	facts := extractionPrepFacts(t, extractionPrepContext())
	if len(facts) != 4 {
		t.Fatalf("len(facts) = %d, want 4", len(facts))
	}
	wantKeys := []string{
		goldenStableKeySourceInstance,
		goldenStableKeyDashboard,
		goldenStableKeyRule,
		goldenStableKeyWarning,
	}
	for i, want := range wantKeys {
		if facts[i].StableKey != want {
			t.Errorf("fact[%d] StableKey = %q, want in-tree golden %q", i, facts[i].StableKey, want)
		}
	}
	if got := fingerprint("Checkout overview"); got != goldenTitleFingerprint {
		t.Errorf("fingerprint(title) = %q, want in-tree golden %q", got, goldenTitleFingerprint)
	}
	payloadBytes, err := json.Marshal(facts[1].Payload)
	if err != nil {
		t.Fatalf("json.Marshal(dashboard payload) error = %v", err)
	}
	if string(payloadBytes) != goldenDashboardPayload {
		t.Errorf("dashboard payload bytes differ from in-tree golden:\n got: %s\nwant: %s", string(payloadBytes), goldenDashboardPayload)
	}
}
