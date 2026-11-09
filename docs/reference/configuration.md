# Configuration reference

There is no config file in this repo. Your host builds Go values and passes them to `grafana.NewClaimedSource`. This page is the field-by-field table for those values; the wiring walkthrough is the [usage guide](../guides/usage.md).

## SourceConfig, field by field

You describe the instance once, then list the Grafana targets it may read.

| Field | What to put there |
| --- | --- |
| `CollectorInstanceID` | Required. This identity gets stamped into fact payloads, and each claim must match it. |
| `Targets` | Your `TargetConfig` list. At least one target needs `Enabled: true`. |
| `ClientFactory` | Optional. Leave it nil and you get the built-in HTTP client per target. Set it when your tests want a fake. |
| `Now` | Optional clock override. Nil means `time.Now`, which is what you want outside tests. |
| `Metrics` | Optional `MetricsRecorder`. The library records counts here and your host turns them into signals. `DiscardMetrics()` keeps things quiet. |

## TargetConfig, field by field

The claim's scope picks the target, so give each target a scope ID you'll keep. Scope and instance IDs end up inside stable fact keys, and changing them later makes the same Grafana look like a brand-new source.

| Field | What to put there |
| --- | --- |
| `Provider` | Leave it empty or set `grafana`. Empty becomes `grafana`, and anything else fails validation. |
| `ScopeID` | Required. A claim whose scope doesn't match a configured target comes back as a retryable failure with no facts. |
| `InstanceID` | Required. Recorded as the fact source instance. |
| `BaseURL` | Required. The Grafana origin. Trailing slashes get trimmed for you. |
| `Token` | Required, and it's the token value itself. Read the next section before you wire this. Validation rejects an empty token. |
| `ResourceLimit` | Cap on records per resource class. Zero picks the default of 100, and the ceiling is 500. Hits turn into `truncated` warnings instead of silent loss. |
| `StaleAfter` | Evidence older than this window gets marked stale. |
| `Enabled` | Disabled targets get ignored before any client is built, duplicates check and all. |
| `DeclaredUIDs` | The set of UIDs you consider declared elsewhere. A live resource in this set earns `matched_declared`, and the rest stay `not_compared`. |
| `ObservedOnlyHint` | Set it when unmatched live resources should read as likely hand-made in the UI. The fetch then flags them `ManuallyCreated`, which surfaces as a manual-provider-resource note in the fact. Matched resources stay untouched. |

Duplicate `ScopeID` values across enabled targets fail construction, and so does a config with nothing enabled. It's better to find that out at startup than at first claim.

## The coded API at a glance

| Name | What it does |
| --- | --- |
| `grafana.NewClaimedSource(SourceConfig)` | Validates config and builds the claim-driven source. Disabled targets are skipped before any client exists. |
| `ClaimedSource.Collect(ctx, claim, grant)` | Runs one claim: grant check, claim checks, bounded fetch, one `collector.Result` or a classified failure. |
| `grafana.NewHTTPClient(HTTPClientConfig)` | The built-in bounded REST client. Sends the token as a Bearer header; sticks to the three GET endpoints in [Facts](facts.md). |
| `EvidenceClient` | The seam your tests can fake (`CollectObservedMetadata`). |
| `MetricsRecorder` / `grafana.DiscardMetrics()` | The seam your host wires to its own signals, or a quiet no-op. |
| `New*Envelope` constructors | Turn bounded evidence into wire facts, if you assemble results yourself. |
| `SchemaVersionForKind` / `CheckSchemaVersion` | Enforce the locked `1.0.0` schema map; unknown kinds and foreign versions fail closed. |
| `grant.Check(grant, now)` / `grant.CoveredKinds()` / `grant.ErrDenied` | The emission-edge grant check, a copy of the allowed kind set, and the denial every failure wraps. |
