# Using the Grafana library from your host

There's no config file to copy here and no binary to launch. Your host builds Go values, hands them over, and calls in once per claim. This page walks that wiring without skipping the fiddly bits.

## Get the module

New to the library? [Getting started](../getting-started/README.md) covers the first import and first call. This guide assumes that much and goes deeper.

Field-by-field tables now live in [Configuration](../reference/configuration.md), and SDK pins in [Facts](../reference/facts.md).

```bash
go get github.com/eshu-hq/eshu-collector-grafana
```

```go
import (
    "github.com/eshu-hq/eshu-collector-grafana/grafana"
    "github.com/eshu-hq/eshu-collector-grafana/grant"
)
```

Build the source once with `grafana.NewClaimedSource(grafana.SourceConfig{...})`, then call `Collect(ctx, claim, grant)` once per claim. A good call returns one `collector.Result` from the Eshu SDK. A provider failure returns a classified `transport.ProviderFailure`, and your host decides whether the claim retries or stops. Don't expect the library to make that call for you, because it won't.



## Handling the token, honestly

Here's the part people trip over, so let's be direct. `TargetConfig.Token` holds the Grafana API token value itself, in Go, passed in by your host. This library defines no `*_env` setting and reads no environment variables for you. If you want the token to come from an env var, a file, or a vault, wire that in your host and pass the resolved string in. The default client factory forwards it into `NewHTTPClient`, which sends it as an `Authorization: Bearer` header.

The defensive side is handled for you: the token never lands in a fact, a log line, or a status. Still, treat the struct as secret-bearing while it lives in memory, and don't print configs wholesale. Give the token only the read access the three GET endpoints need (`/api/search`, `/api/datasources`, `/api/v1/provisioning/alert-rules`). This repo doesn't name Grafana roles or token scopes, because the code doesn't define them.


## Checks before you ship a change here

```bash
make ci     # lint + test + race, the full local check
make lint   # vet, golangci-lint, format diff, actionlint, tidy diff, 500-line cap
make test   # go build ./... and go test ./... -count=1
make race   # race tests plus the coverage floor (91.5%)
```

CI in `.github/workflows/ci.yml` stays the authority, and `make ci` is your fast local floor before push. The `image` lane builds the `Dockerfile` (no push), and `.github/workflows/image-publish.yml` publishes tagged releases; both are pinned by `ci_image_lane_test.go`. Two test files deserve your respect when you touch fetch or envelope code: `grafana/extraction_prep_mirror_test.go` and `grafana/extraction_prep_golden_test.go` pin parity with the in-tree producer this library came from, and `TestCollectEmitsValidatorCleanResult` runs results through the published SDK validator.
