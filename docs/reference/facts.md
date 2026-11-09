# Facts reference

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

## The three Grafana reads

| Endpoint | Feeds |
| --- | --- |
| `GET /api/search` | folders and dashboards |
| `GET /api/datasources` | datasources |
| `GET /api/v1/provisioning/alert-rules` | alert rules |

Facts stick to metadata: names, IDs, presence flags, and SHA-256 fingerprints. Dashboard JSON, datasource URLs, query text, contact destinations, and credentials stay out.

## Which SDK versions you're pinned to

Exact pins from `go.mod`, with no `replace` directives. If these ever drift from the file, trust the file and fix this page.

| Module | Pin |
| --- | --- |
| `github.com/eshu-hq/eshu-sdk/collector` | `v0.1.0-rc.3` |
| `github.com/eshu-hq/eshu-sdk/factschema` | `v0.1.0-rc.5` |

The module itself is `github.com/eshu-hq/eshu-collector-grafana` and `go.mod` asks for Go `1.26.10`. One known gap to keep in mind: the pinned `factschema` has no per-kind schema-version helper yet, so the locked `1.0.0` map lives in `grafana/schema.go` until the SDK grows one. Don't widen that map on your own.
