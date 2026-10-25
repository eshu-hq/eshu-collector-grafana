// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var fallbackClaimSequence uint64

// newClaimID mints a producer-namespaced claim identity. Consecutive IDs
// are unique per acquisition; the grafana-claim- prefix namespaces the
// claim to this producer family.
func newClaimID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "grafana-claim-" + hex.EncodeToString(raw[:])
	}
	next := atomic.AddUint64(&fallbackClaimSequence, 1)
	return fmt.Sprintf("grafana-claim-fallback-%d-%d", time.Now().UTC().UnixNano(), next)
}
