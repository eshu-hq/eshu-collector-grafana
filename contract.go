// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	sdk "github.com/eshu-hq/eshu-sdk/collector"

	"github.com/eshu-hq/eshu-collector-grafana/grafana"
	"github.com/eshu-hq/eshu-collector-grafana/grant"
)

const (
	// ComponentID is the manifest identity of this collector. It is the
	// grant producer identity the host admits; keep it in sync with
	// metadata.id in manifest.yaml.
	ComponentID = grant.ProducerID
	// CollectorKind is the durable collector family identifier for live
	// Grafana metadata facts.
	CollectorKind = grafana.CollectorKind
	// SourceSystem is the source system name facts carry when the claim
	// does not name one.
	SourceSystem = grafana.CollectorKind
	// MetricsPrefix is the component-owned metric prefix the manifest
	// declares. Span and counter names keep the in-tree eshu_dp_grafana_*
	// vocabulary.
	MetricsPrefix = "eshu_dp_grafana_"
	// SchemaVersion is the schema version of every emitted Grafana fact.
	SchemaVersion = grafana.SchemaVersionV1
	// GrantVersion is the collector artifact version the producer grants
	// bind. Keep it in sync with metadata.version in manifest.yaml.
	GrantVersion = grant.Version
)

// FactKinds returns every fact kind this collector may emit, in the order
// the manifest declares them. It mirrors the grant coverage so the
// entrypoint cannot emit a kind the grant does not cover.
func FactKinds() []string {
	return grant.CoveredKinds()
}

// Contract returns the SDK fact families this collector may emit.
func Contract() sdk.Contract {
	kinds := FactKinds()
	facts := make([]sdk.FactDeclaration, 0, len(kinds))
	for _, kind := range kinds {
		facts = append(facts, sdk.FactDeclaration{
			Kind:             kind,
			SchemaVersions:   []string{SchemaVersion},
			SourceConfidence: []sdk.SourceConfidence{sdk.SourceConfidenceReported},
		})
	}
	return sdk.Contract{ProtocolVersion: sdk.ProtocolVersionV1Alpha1, Facts: facts}
}
