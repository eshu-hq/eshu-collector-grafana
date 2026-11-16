// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grant

import (
	"testing"
	"time"
)

func validTestGrant() Grant {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	return Grant{
		ProducerID: ProducerID,
		Version:    Version,
		Scope:      Scope,
		Kinds: []KindGrant{
			{Kind: "observability.source_instance", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.observed_dashboard", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.observed_rule", SchemaVersions: []string{"1.0.0"}},
			{Kind: "observability.coverage_warning", SchemaVersions: []string{"1.0.0"}},
		},
		IssuedAt:  now,
		ExpiresAt: now.Add(720 * time.Hour),
	}
}

func TestCheckAcceptsDraftShapedGrant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if err := Check(validTestGrant(), now); err != nil {
		t.Fatalf("Check(valid) error = %v, want nil", err)
	}
}

// TestCheckAcceptsHostShapedGrant pins the live-ops-qa regression: the host
// grant registry records expires_at but no issued_at, so a delivered grant
// with zero IssuedAt and a future expiry must pass (issuance checks are
// skipped, not failed). A grant that is expired must still deny.
func TestCheckAcceptsHostShapedGrant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 16, 11, 10, 0, 0, time.UTC)
	delivered := validTestGrant()
	delivered.IssuedAt = time.Time{}
	delivered.ExpiresAt = now.Add(720 * time.Hour)
	if err := Check(delivered, now); err != nil {
		t.Fatalf("Check(host-shaped, no issued_at) error = %v, want nil", err)
	}
	delivered.ExpiresAt = now.Add(-time.Second)
	if err := Check(delivered, now); err == nil {
		t.Fatal("Check(host-shaped, expired) = nil, want fail-closed denial")
	}
}

func TestCheckFailsClosed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	valid := validTestGrant()

	tests := []struct {
		name   string
		mutate func(*Grant)
	}{
		{"zero grant denies", func(g *Grant) { *g = Grant{} }},
		{"wrong producer denies", func(g *Grant) { g.ProducerID = "dev.eshu.collector.other" }},
		{"wrong version denies", func(g *Grant) { g.Version = "9.9.9" }},
		{"wrong scope denies", func(g *Grant) { g.Scope = "prometheus" }},
		{"missing kind denies", func(g *Grant) { g.Kinds = g.Kinds[:3] }},
		{"extra kind denies", func(g *Grant) {
			g.Kinds = append(g.Kinds, KindGrant{Kind: "observability.declared_dashboard", SchemaVersions: []string{"1.0.0"}})
		}},
		{"wrong schema denies", func(g *Grant) { g.Kinds[1].SchemaVersions = []string{"2.0.0"} }},
		{"expired denies", func(g *Grant) { g.ExpiresAt = now.Add(-time.Second) }},
		{"missing expiry denies", func(g *Grant) { g.ExpiresAt = time.Time{} }},
		{"non-positive lifetime denies", func(g *Grant) { g.ExpiresAt = g.IssuedAt }},
		{"overlong lifetime denies", func(g *Grant) { g.ExpiresAt = g.IssuedAt.Add(721 * time.Hour) }},
		{"future-issued denies", func(g *Grant) { g.IssuedAt = now.Add(time.Hour) }},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			// Deep-copy the kind slice so mutations stay case-local.
			candidate.Kinds = append([]KindGrant(nil), valid.Kinds...)
			tt.mutate(&candidate)
			if err := Check(candidate, now); err == nil {
				t.Errorf("Check() = nil, want fail-closed denial for %s", tt.name)
			}
		})
	}
}

func TestCoveredKindsCopyCannotWeakenCheck(t *testing.T) {
	t.Parallel()

	mutated := CoveredKinds()
	mutated[0] = "observability.declared_dashboard"
	mutated = append(mutated, "observability.extra_kind")
	if len(mutated) != len(coveredKinds)+1 {
		t.Fatalf("len(mutated copy) = %d, want %d", len(mutated), len(coveredKinds)+1)
	}

	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if err := Check(validTestGrant(), now); err != nil {
		t.Fatalf("Check(valid) after mutating the CoveredKinds() copy = %v, want nil (copy must not affect enforcement)", err)
	}
	if got := CoveredKinds(); got[0] != "observability.source_instance" || len(got) != 4 {
		t.Fatalf("CoveredKinds() = %q, want the intact 4-kind set", got)
	}
}
