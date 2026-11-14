// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

// Package collector is the satellite packaging of the extracted Grafana
// collector: the stdin/stdout entrypoint wire-up, the component manifest,
// and the host-delivered grant path.
//
// With no arguments (or --sdk-stdio) the cmd/collector binary serves one
// claim: it reads one collector-sdk/v1alpha1 request from stdin — claim,
// activation config, and the host-delivered core-issued producer grant —
// drives one grafana.ClaimedSource collection, and writes one result to
// stdout. The grant travels with the request because this satellite has no
// grant registry of its own; grant.Check fails closed on a zero grant, so
// an unwired host gets a terminal result with zero facts, never evidence.
//
// Extracted from eshu-hq/eshu go/internal/collector/observability/grafana
// (#8857). The claim-driven source lives in the grafana package and the
// grant edge in the grant package; this package only adapts the host wire
// to those two seams.
package collector
