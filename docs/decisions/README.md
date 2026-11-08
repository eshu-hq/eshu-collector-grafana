# Decisions

This folder holds architecture decision records (ADRs) for eshu-collector-grafana: short notes that capture one decision, why it was made, and what it costs us. They save future readers from re-litigating settled questions.

## Index

No architecture decision records are recorded in this repo yet. Decisions that live in code comments, the manifest, or Eshu core issues are linked from the pages that rely on them.

Provenance that is recorded in code, for the curious: this library was extracted from the in-tree producer in `eshu-hq/eshu` (`go/internal/collector/observability/grafana`, #8857), and the grant draft terms live with core (#6709, #4047). Those are references, not ADRs in this repo.

## Format

Name files `NNNN-short-slug.md`, numbered in order, and keep each one to this shape:

```markdown
# ADR-NNNN: Short title

Status: Proposed | Accepted | Superseded by ADR-MMMM
Date: YYYY-MM-DD

## Context

What forced a decision, in plain terms.

## Decision

What we chose.

## Consequences

What gets easier, what gets harder, and what we must now keep true.
```

Add a row to the index above in the same change. An ADR is only added for a decision that is actually recorded — do not back-fill ADRs for choices nobody wrote down.
