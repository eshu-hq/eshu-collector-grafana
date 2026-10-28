// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"testing"
)

// TestSetRedactionStateSignals pins each signal branch with a single-key
// payload. The implementation ranges over a map, so a multi-signal payload
// covers whichever branch the iteration order yields first and the total
// flickers between runs; one signal per call keeps every branch covered on
// every run.
func TestSetRedactionStateSignals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"redacted flag applies", map[string]any{"url_redacted": true}, "applied"},
		{"false flag is not a signal", map[string]any{"url_redacted": false}, "none"},
		{"fingerprint applies", map[string]any{"title_fingerprint": "sha256:abc"}, "applied"},
		{"plain fields leave none", map[string]any{"title_present": true}, "none"},
		{"empty payload leaves none", map[string]any{}, "none"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			setRedactionState(tt.payload)
			if tt.payload["redaction_state"] != tt.want {
				t.Errorf("redaction_state = %v, want %q", tt.payload["redaction_state"], tt.want)
			}
		})
	}
}
