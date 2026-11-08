# Getting started

This repo is a Go library, not a program — there is nothing to run. Your host (inside Eshu core) imports two packages, builds a source once, and calls it once per claim. This page takes you from an empty module to your first result.

## 1. Add the module

```bash
go get github.com/eshu-hq/eshu-collector-grafana
```

## 2. Import the two packages

```go
import (
    "github.com/eshu-hq/eshu-collector-grafana/grafana"
    "github.com/eshu-hq/eshu-collector-grafana/grant"
)
```

`grafana` does the collecting. `grant` checks the core-issued producer grant your host must pass with every call.

## 3. Build the source once

```go
source, err := grafana.NewClaimedSource(grafana.SourceConfig{
    CollectorInstanceID: "your-instance-id",
    Targets: []grafana.TargetConfig{{
        ScopeID:    "grafana:example",
        InstanceID: "example",
        BaseURL:    "https://grafana.example.test",
        Token:      tokenFromYourSecretStore, // the value itself; see the token notes in the usage guide
        Enabled:    true,
    }},
})
```

Construction validates as it goes: at least one target must be enabled, duplicate `ScopeID` values fail, and `Provider` must be empty or `grafana`. Better to learn that at startup than at first claim. Every field is spelled out in [Configuration](../reference/configuration.md).

## 4. Call it once per claim

```go
result, err := source.Collect(ctx, claim, grantValue)
```

The grant is checked first, before anything talks to Grafana — a missing or wrong grant fails the call with no provider contact, which is exactly what you want. A good call returns one `collector.Result`; a provider failure returns a classified `transport.ProviderFailure`, and your host decides whether the claim retries or stops.

## Where next

- Wiring details, token handling, and test seams: [Using the library from your host](../guides/usage.md).
- What comes back: [Facts](../reference/facts.md).
- The full flow: [Architecture](../architecture/README.md) and [Diagrams](../diagrams/README.md).
