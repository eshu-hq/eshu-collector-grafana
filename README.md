# eshu-collector-grafana

Status: pre-release library. It is not yet the live Grafana producer for Eshu, and there is no program here to run on its own.

This repository is a Go library that reads live Grafana metadata and turns it into Eshu facts (small, checked records Eshu stores as evidence).

## No binary to run

This repo ships Go packages only. It has no `cmd/` program, no `Dockerfile`, no `manifest.yaml`, and no example activation file.

That means:

- There is no collector binary to start.
- There is no container image to pull from this repo.
- There is no seed, install, or activation step in this repo.

A host program in Eshu core imports this library, passes it one work request (a *claim*: one bounded unit of collection work), and receives one result. Deployment and scheduling belong to that host, not here.

```mermaid
flowchart LR
    G["Grafana API"] --> L["This library (imported)"]
    H["Eshu core host"] -->|"one claim"| L
    L -->|"one result with facts"| H
```

In words: the host hands this library one claim, the library reads Grafana, and the host receives facts to store. Nothing here runs on a schedule by itself.

## What the packages emit

The `grafana` package emits 4 fact kinds. Each kind uses schema version `1.0.0` and source confidence `reported` (the provider itself reported the value).

| Fact kind | What it records | Grafana read behind it |
| --- | --- | --- |
| `observability.source_instance` | One configured Grafana target | The configured target itself |
| `observability.observed_dashboard` | One folder, dashboard, or datasource seen live | `GET /api/search`, `GET /api/datasources` |
| `observability.observed_rule` | One live alert rule | `GET /api/v1/provisioning/alert-rules` |
| `observability.coverage_warning` | A gap: hidden, throttled, stale, partial, or truncated data | Any read above that could not finish cleanly |

Facts hold metadata only (names, IDs, presence flags, and SHA-256 fingerprints — short hashes that prove two values match without storing the value). Dashboard JSON, datasource URLs, query models, contact destinations, notification routes, and credentials never enter a fact.

## Packages and public API

| Package | Import path | Main public surface |
| --- | --- | --- |
| `grafana` | `github.com/eshu-hq/eshu-collector-grafana/grafana` | `NewClaimedSource`, `ClaimedSource.Collect`, `NewHTTPClient`, the four `New*Envelope` constructors, `SchemaVersionForKind`, `CheckSchemaVersion` |
| `grant` | `github.com/eshu-hq/eshu-collector-grafana/grant` | `Check`, `CoveredKinds`, `Grant`, `KindGrant`, `ErrDenied` |

Key types in `grafana`:

- `SourceConfig` and `TargetConfig` describe the collector instance and each Grafana target.
- `EvidenceClient` is the fetch seam; `HTTPClient` is the built-in bounded REST client.
- `MetricsRecorder` is a host-wired seam. The library records counts against it; the host turns those counts into its own signals. `DiscardMetrics()` is the no-op recorder.
- `EnvelopeContext`, `Resource`, `AlertRule`, `Warning`, and `CollectionResult` carry the bounded evidence between fetch and fact construction.

The `grant` package checks a core-issued producer grant (permission for this producer to emit core-owned kinds) at every emission. The check covers producer identity (`dev.eshu.collector.grafana`), version (`0.1.0`), scope (`grafana`), the exact 4-kind set at schema `1.0.0`, and expiry (maximum lifetime 720 hours). A missing or mismatched grant fails closed (stops safely) before any Grafana contact. The check verifies shape and expiry only; it does not verify a core signature, because no signature scheme exists yet.

## Use it from Go

```bash
go get github.com/eshu-hq/eshu-collector-grafana
```

```go
import (
    "github.com/eshu-hq/eshu-collector-grafana/grafana"
    "github.com/eshu-hq/eshu-collector-grafana/grant"
)
```

Build a source with `grafana.NewClaimedSource(grafana.SourceConfig{...})`, then call `Collect(ctx, claim, grant)` once per claim. `Collect` returns one `collector.Result` (from the Eshu SDK) whose facts match the table above, or a classified provider failure the host can act on. The library does not decide retry timing itself.

## Configuration (Go values, not a file)

There is no config file in this repo. The importing host builds these values in code:

| Field | Meaning |
| --- | --- |
| `SourceConfig.CollectorInstanceID` | Required. Identity stamped into fact payloads. |
| `SourceConfig.Targets` | At least one enabled target is required. |
| `TargetConfig.ScopeID`, `InstanceID`, `BaseURL` | Required per target. The claim scope selects the target. |
| `TargetConfig.Token` | Grafana API token value, passed in by the host. This library defines no `*_env` setting; how the host loads the token is host-owned. |
| `TargetConfig.ResourceLimit` | Cap on records per resource class. Hits become `truncated` warnings. |
| `TargetConfig.StaleAfter` | Evidence older than this is marked stale. |
| `TargetConfig.Enabled` | Disabled targets are ignored before any client is built. |
| `TargetConfig.DeclaredUIDs`, `ObservedOnlyHint` | Hints for declared-vs-observed comparison, which core owns. |

Give the token only the read access the three `GET` endpoints above need. This repo does not name Grafana roles or token scopes, because the code does not define them.

## When to use it

Use this library when you build or test the Eshu host path that collects live Grafana dashboards, datasources, folders, and alert rules as evidence.

Do not use it as a standalone Grafana exporter, dashboard backup tool, or provisioning tool. It never writes to Grafana, never stores dashboard content, and cannot run without a host.

## What it never does

- It never persists anything itself: no database, graph, queue, or lease handle.
- It never emits declared Grafana kinds. Declared evidence stays with core and GitOps sources; this library covers live observed evidence only.
- It never decides admission, projection, persistence, claim mutation, or fencing (guarding against stale writers). Eshu core owns those steps.
- It never puts tokens, URLs with credentials, query text, or provider response bodies into facts.

## Eshu dependencies

Exact pins from `go.mod` in this repo. No `replace` directives.

| Module | Pin |
| --- | --- |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.1.0-rc.3` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.5` |

Go version in `go.mod`: `1.26.10`. Module path: `github.com/eshu-hq/eshu-collector-grafana`.

One known gap: the pinned `factschema` has no per-kind schema-version helper, so the locked `1.0.0` map lives in `grafana/schema.go` until the SDK provides one.

## Build and test

```bash
make ci     # lint + test + race, the full local check
make lint   # vet, golangci-lint, format diff, actionlint, tidy diff, 500-line cap
make test   # go build ./... and go test ./... -count=1
make race   # race tests plus coverage floor (91.5%)
```

CI (`.github/workflows/ci.yml`) is the authority; `make ci` is the fast local floor before push. There is no release or image workflow in this repo, because there is no binary or image to publish.

Tests worth knowing: `grafana/extraction_prep_mirror_test.go` and `grafana/extraction_prep_golden_test.go` pin parity with the in-tree producer this library was extracted from, and `TestCollectEmitsValidatorCleanResult` runs results through the published SDK validator.

## Docs and related repos

- [eshu](https://github.com/eshu-hq/eshu) — Eshu core: admission, storage, claims, and the host that will import this library.
- [eshu-sdk](https://github.com/eshu-hq/eshu-sdk) — the collector and fact-schema modules pinned above.
- [eshu-deploy](https://github.com/eshu-hq/eshu-deploy) — deployment for Eshu components. Nothing in this repo deploys itself.

Local docs: [CONTRIBUTING.md](CONTRIBUTING.md) covers local checks and the coverage floor. Package docs live in `grafana/doc.go` and the package comment in `grant/grant.go`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: run `make ci` before pushing, keep Go files under 500 lines, and do not lower the coverage floor. This repo has no `SECURITY.md` and no `LICENSE` file; source files carry an SPDX MIT header.
