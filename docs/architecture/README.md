# Architecture

You're the host author here, so let's walk the path your code actually drives. This repo has no `cmd/` program, no `Dockerfile`, no `manifest.yaml`, and no activation file. Your host imports two packages and does the driving. Scheduling, admission, storage, and retries stay on your side of the fence.

## The trip a claim takes

See [Diagrams](../diagrams/README.md) for the picture.

In words: your host hands over a claim and a grant, the library checks the grant, picks the target the claim names, fetches bounded metadata, wraps it as facts, and returns one `collector.Result`. If Grafana misbehaves, you get a classified failure instead, and you decide what happens next. The library doesn't pick retry timing for you.

Here's the grant check up close, because it happens first and it can stop everything:

See [Diagrams](../diagrams/README.md) for the picture.

Text version, step by step: the host calls `Collect`, `Collect` asks `grant.Check` to validate the grant, and a bad grant ends the call right there. Only a good grant leads to target matching and Grafana reads. A good read comes back as a result full of facts.

## What comes out

The `grafana` package emits four kinds of fact. Each one carries schema version `1.0.0` and source confidence `reported`, which just means Grafana itself told us the value.

| Kind | You'll see one per… | Which read feeds it |
| --- | --- | --- |
| `observability.source_instance` | configured Grafana target | the target you configured |
| `observability.observed_dashboard` | folder, dashboard, or datasource seen live | `GET /api/search`, `GET /api/datasources` |
| `observability.observed_rule` | live alert rule | `GET /api/v1/provisioning/alert-rules` |
| `observability.coverage_warning` | gap we want you to know about | any read above that hit a limit or a snag |

A warning covers the honest imperfections: data your token can't see, throttling, stale evidence, a partial read, or a resource class that hit its cap. When a read comes back partial or truncated, the result says so with a partial state and a warning status. You'll never mistake a clipped snapshot for a complete one.

Facts stick to metadata: names, IDs, presence flags, and SHA-256 fingerprints. A fingerprint proves two values match without storing the value itself, which is handy for queries and contact points you shouldn't persist. Dashboard JSON, datasource URLs, query models, contact destinations, notification routes, and credentials stay out. Always.

## The two packages, up close

The `grafana` package does the collecting. Its package comment in `grafana/doc.go` puts the boundary well: it reads configured targets and emits metadata-only facts for the `collector-sdk/v1alpha1` wire contract, while core owns admission, projection, persistence, claim mutation, and fencing (the guard against stale writers). The package was extracted from the in-tree producer in `eshu-hq/eshu` (`go/internal/collector/observability/grafana`, #8857), and it leans on the pinned SDK modules for payload encoding, HTTP transport, and failure classification.

Names you'll touch most:

- `NewClaimedSource` validates your config and builds the source. Disabled targets get skipped before any client exists, and you need at least one enabled target or construction fails.
- `ClaimedSource.Collect` runs one claim end to end: grant first, then claim checks (instance ID, collector kind, source system `grafana`, generation ID), then the fetch.
- `NewHTTPClient` builds the built-in bounded REST client. It sends your token as a Bearer header and sticks to the three GET endpoints in the table.
- The four `New*Envelope` constructors turn bounded evidence into wire facts. You'll rarely call them directly, but they're there if you assemble results yourself.
- `SchemaVersionForKind` and `CheckSchemaVersion` enforce the locked `1.0.0` map in `grafana/schema.go`. Unknown kinds and foreign versions fail closed.

A few types carry data between fetch and fact: `Resource` for a folder, dashboard, or datasource, `AlertRule` for a rule, `Warning` for a coverage gap, and `CollectionResult` for the whole snapshot plus counters. `EvidenceClient` is the seam your tests can fake, and `MetricsRecorder` is the seam your host wires to its own signals. `DiscardMetrics()` gives you a quiet no-op when you don't care.

The `grant` package guards the exit. Roads in Eshu are picky about who may emit core-owned kinds, so `Check` verifies the whole bundle at every emission: producer `dev.eshu.collector.grafana`, version `0.1.0`, scope `grafana`, exactly those four kinds at schema `1.0.0`, and a sane lifetime (issued in the past, expiring in the future, never longer than 720 hours). Extra kinds fail too, because declared Grafana evidence belongs to core and GitOps sources, not here. One honest caveat from the code: this checks shape and expiry only. There's no signature verification yet, since core hasn't defined a signature scheme. `CoveredKinds` hands you a copy of the allowed set, and every denial wraps `ErrDenied` so you can test for it cleanly.

## Lines this library won't cross

It helps to know the refusals up front. This library never persists anything, so there's no database, graph, queue, or lease handle hiding inside. It never emits declared kinds, never decides admission or projection, and never puts tokens, credential-bearing URLs, query text, or provider response bodies into a fact, a log line, or a status. If you need any of that, you're in the wrong repo, and core is the place to look.
