# eshu-collector-grafana

Extracted Grafana collector producer for [eshu-hq/eshu#8857](https://github.com/eshu-hq/eshu/issues/8857)
(collector-grafana extraction, part of epic #6707). Emission half only:
this repository fetches bounded live Grafana metadata and returns a
`collector-sdk/v1alpha1` Result per host claim. Core keeps admission,
projection, persistence, claim mutation, fencing, and the status read.

Refs #8857. This repository does not close that issue: cutover stays gated
by #4047 with production acceptance NOT_CHECKED, and no pair is a
qualified consumer/core combination yet (see SDK Compatibility in
eshu-hq/eshu docs).

## Pins

Exact published pins from `eshu-hq/eshu-sdk` — never `@latest`, no
`replace` or workspace directives:

| Module | Pin |
|---|---|
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.1.0-rc.3` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.5` |

Verify resolution before any pin bump:

```bash
GOPROXY=proxy.golang.org go list -m github.com/eshu-hq/eshu-sdk/collector@v0.1.0-rc.3
GOPROXY=proxy.golang.org go list -m github.com/eshu-hq/eshu-sdk/factschema@v0.1.0-rc.5
```

## Layout

- `grafana/` — the producer: bounded REST fetch (`client.go`), four
  envelope constructors (`envelope.go`), claim-to-Result fan-out
  (`source.go`), metadata-only redaction (`redaction.go`), kind/schema
  admission (`schema.go`), claim-ID minting (`claimid.go`), and a
  host-wired metrics seam (`metrics.go`).
- `grant/` — fail-closed stub enforcing the core-issued producer grant
  draft (`producer-grant.manifest.DRAFT.yaml` in eshu-hq/eshu,
  `go/cmd/collector-grafana`) at every emission. Until core issues a
  grant, the zero Grant is denied and `Collect` fails closed before any
  provider contact.

## Parity evidence

- Census mirror: `grafana/extraction_prep_mirror_test.go` ports the
  in-tree extraction PREP mirror (admission at 1.0.0, determinism,
  generation rebind, redaction sentinel, fail-closed hostile inputs).
  `TestClaimIDNamespace` ports the `grafana-claim-` namespace case.
- Stable keys: `kind:generationID:identity.StableID(kind, identity)` is
  byte-stable with the in-tree scheme. Independent oracle: Python
  `sha256('{"fact_type":"observability.source_instance","identity":...}')`
  yields `4c1e93a9…0b723a9`, identical to the SDK helper output.
- Contract payloads: `observability/v1` struct fields are identical
  between the in-tree `sdk/go/factschema` source and the pinned module,
  and the same `EncodeObservability*` entry points merge payloads.
- SDK self-check: `TestCollectEmitsValidatorCleanResult` and
  `TestCollectPartialReadStaysPartial` run emitted Results through the
  published SDK's own `collector.NewValidator(...).ValidateResult`,
  including its sensitive-payload-key gate.
- In-tree `FactID` (host-assigned at admission) and numeric
  `FencingToken` (host-enforced from `claim.fencing_token`) have no
  emission-half counterpart and are intentionally not ported.

## Boundaries (core-owned, not ported)

Instance selection, the heartbeat-under-lease guard, the Postgres commit
boundary, cassette replay, and `eshu_dp_grafana_*` signals stay in core.
Telemetry records against the local `MetricsRecorder` seam; the host
adapts it to its own signals. `source_system` binding is strict
(`grafana` exactly): the SDK host validator requires fact source_system
to equal the claim's, so anything else fails closed at the edge.

## Open gaps

- P1: the published factschema `v0.1.0-rc.5` has no per-kind
  schema-version map or `ValidateSchemaVersion` admission helper
  (compare `ObservabilitySchemaVersion` / `ValidateSchemaVersion` in
  eshu-hq/eshu `go/internal/facts`). The locked `1.0.0` map lives in
  `grafana/schema.go` until the SDK gains one.
- P1: `grant.Check` verifies shape, identity, scope, kind coverage,
  schema versions, and expiry only — no core issuance-signature
  verification exists yet (`grant/grant.go`). Issuance is core-owned.
- Note: no `SHIM_MAP.md` exists in `go/cmd/collector-grafana`; the shim
  mapping used here was derived from `EXTRACTION_PREP.md` plus the
  in-tree `collector/sdk` sources and is recorded in the PR body.

## Deferred to the host (NOT_CHECKED here)

The following in-tree surface has no counterpart in this repository by
design; the host owns it at cutover (tracked on eshu-hq/eshu#8857):

- `cmd/` entrypoint: `main`/`service`/`config` wiring, env-token intake,
  cassette replay, heartbeat-under-lease, and multi-instance fail-closed
  (`EXTRACTION_PREP.md` §1). This repo ships the emission library only.
- OTEL span seam: in-tree `startObserve`/`startFetch` spans are not
  ported; telemetry records against the local `MetricsRecorder` seam
  (`metrics.go`) and the host adapts it to its own signals.
- `newClaimID` (`claimid.go`) is exercised only by its unit test here;
  the host service wires it to claim acquisition (in-tree `service.go:82`
  equivalent). `EnvelopeContext.FencingToken` is likewise host-set from
  `claim.fencing_token`.

<!-- probe-docs: lane-selection check, will be discarded -->
