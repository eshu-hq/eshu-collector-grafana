// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"strings"
	"testing"
)

// Ported from the claim-ID namespace case of
// go/cmd/collector-grafana/extraction_prep_mirror_test.go
// (TestExtractionPrepClaimIDNamespace) in eshu-hq/eshu. Instance selection
// and the heartbeat guard stay core-owned: they govern the workflow control
// store, which this emission-half producer never touches.
func TestClaimIDNamespace(t *testing.T) {
	t.Parallel()

	first, second := newClaimID(), newClaimID()
	for _, id := range []string{first, second} {
		if !strings.HasPrefix(id, "grafana-claim-") {
			t.Errorf("claim ID %q missing grafana-claim- namespace prefix", id)
		}
	}
	if first == second {
		t.Error("consecutive claim IDs identical; claim identity must be unique per acquisition")
	}
}
