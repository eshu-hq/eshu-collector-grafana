# Concepts

## What this is

A pre-release Go library that reads live Grafana metadata — dashboards, folders, datasources, alert rules — and hands Eshu small checked records (facts) it can store as evidence. A host inside Eshu core imports it and drives it one bounded work request (a claim) at a time. It is not the live Grafana producer for Eshu yet.

## Why it exists

Eshu wants to answer "what does that Grafana actually serve right now?" from live evidence, not from what a config file promised. Live reads catch dashboards made by hand in the UI and rules that drifted from their declared form.

## The vocabulary that matters

- **Claim** — one bounded unit of work from the host: a collector instance, a scope, and a generation.
- **Grant** — core's permission slip for emitting core-owned fact kinds, checked on every call before any provider contact. See [Architecture](../architecture/README.md) and the `grant` package.
- **Observed vs declared** — this library only ever reports what Grafana serves live (observed). What should exist (declared) belongs to core and GitOps sources. `DeclaredUIDs` and `ObservedOnlyHint` only add comparison hints — see [Configuration](../reference/configuration.md).
- **Fingerprint** — a SHA-256 hash that proves two values match without storing the value itself.

## Boundaries it will not cross

It never persists anything (no database, graph, queue, or lease handle), never writes to Grafana, never emits declared kinds, and never decides admission, projection, or retry timing. Dashboard JSON, datasource URLs, query models, contact points, notification routes, tokens, and provider response bodies never enter a fact, a log line, or a status. Scheduling, storage, and claim fate stay with your host and Eshu core.
