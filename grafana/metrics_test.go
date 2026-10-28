// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 eshu-hq

package grafana

import (
	"errors"
	"testing"
	"time"

	"github.com/eshu-hq/eshu-sdk/collector/transport"
)

type recordedFetch struct {
	provider    string
	statusClass string
}

type recordedCount struct {
	provider string
	count    int
}

type recordedFacts struct {
	provider string
	counts   map[string]int64
}

type recordingMetrics struct {
	fetches     []recordedFetch
	rateLimited []recordedCount
	facts       []recordedFacts
	redactions  []recordedCount
	retries     []recordedCount
}

func (r *recordingMetrics) RecordFetch(provider string, statusClass string, _ time.Duration) {
	r.fetches = append(r.fetches, recordedFetch{provider: provider, statusClass: statusClass})
}

func (r *recordingMetrics) RecordRateLimited(provider string, count int) {
	r.rateLimited = append(r.rateLimited, recordedCount{provider: provider, count: count})
}

func (r *recordingMetrics) RecordFactsEmitted(provider string, counts map[string]int64) {
	r.facts = append(r.facts, recordedFacts{provider: provider, counts: counts})
}

func (r *recordingMetrics) RecordRedactions(provider string, count int) {
	r.redactions = append(r.redactions, recordedCount{provider: provider, count: count})
}

func (r *recordingMetrics) RecordRetries(provider string, count int) {
	r.retries = append(r.retries, recordedCount{provider: provider, count: count})
}

func metricsTarget() TargetConfig {
	return TargetConfig{
		Provider: ProviderGrafana,
		ScopeID:  "grafana:prod",
		BaseURL:  "https://grafana.example.test",
		Token:    "test-token",
		Enabled:  true,
	}
}

func metricsSource(recorder MetricsRecorder) *ClaimedSource {
	return &ClaimedSource{collectorInstanceID: "collector-grafana", recorder: recorder}
}

func TestDiscardMetricsAcceptsEverySignal(t *testing.T) {
	t.Parallel()

	recorder := DiscardMetrics()
	if recorder == nil {
		t.Fatal("DiscardMetrics() = nil, want the no-op recorder")
	}
	recorder.RecordFetch("grafana", "success", time.Millisecond)
	recorder.RecordRateLimited("grafana", 1)
	recorder.RecordFactsEmitted("grafana", map[string]int64{KindObservedDashboard: 2})
	recorder.RecordRedactions("grafana", 1)
	recorder.RecordRetries("grafana", 1)
}

func TestMetricsSelectorFallsBackToDiscard(t *testing.T) {
	t.Parallel()

	var nilSource *ClaimedSource
	if nilSource.metrics() == nil {
		t.Error("nil source metrics() = nil, want the discard recorder")
	}
	if metricsSource(nil).metrics() == nil {
		t.Error("nil-recorder metrics() = nil, want the discard recorder")
	}
	recorder := &recordingMetrics{}
	if got := metricsSource(recorder).metrics(); got != MetricsRecorder(recorder) {
		t.Error("metrics() did not return the configured recorder")
	}
}

func TestRecordFetchForwardsProviderAndStatus(t *testing.T) {
	t.Parallel()

	recorder := &recordingMetrics{}
	source := metricsSource(recorder)
	source.recordFetch(metricsTarget(), "success", time.Now().Add(-time.Millisecond))
	if len(recorder.fetches) != 1 {
		t.Fatalf("len(fetches) = %d, want 1", len(recorder.fetches))
	}
	if recorder.fetches[0].provider != ProviderGrafana || recorder.fetches[0].statusClass != "success" {
		t.Errorf("fetch = %+v, want provider grafana with success status", recorder.fetches[0])
	}
}

func TestRecordRateLimitOnlyFiresWhenThrottled(t *testing.T) {
	t.Parallel()

	transient := transport.NewProviderFailure("grafana", transport.FailureRetryable, false, errors.New("boom"))
	throttled := transport.NewProviderFailure("grafana", transport.FailureRateLimited, false, errors.New("slow down"))
	if throttled.FailureClass() != FailureRateLimited {
		t.Fatalf("throttled FailureClass() = %q, want rate_limited", throttled.FailureClass())
	}

	recorder := &recordingMetrics{}
	source := metricsSource(recorder)
	source.recordRateLimit(metricsTarget(), transient)
	if len(recorder.rateLimited) != 0 {
		t.Fatalf("transient failure recorded %d rate-limit signals, want 0", len(recorder.rateLimited))
	}
	source.recordRateLimit(metricsTarget(), throttled)
	if len(recorder.rateLimited) != 1 || recorder.rateLimited[0].count != 1 {
		t.Fatalf("rateLimited = %+v, want one signal with count 1", recorder.rateLimited)
	}
}

func TestRecordGuardsDropNonPositiveCounts(t *testing.T) {
	t.Parallel()

	recorder := &recordingMetrics{}
	source := metricsSource(recorder)
	target := metricsTarget()
	source.recordRateLimits(target, 0)
	source.recordRateLimits(target, -1)
	source.recordRedactions(target, 0)
	source.recordRedactions(target, -2)
	source.recordRetries(target, 0)
	source.recordRetries(target, -3)
	source.recordFacts(target, nil)
	if len(recorder.rateLimited) != 0 || len(recorder.redactions) != 0 || len(recorder.retries) != 0 || len(recorder.facts) != 0 {
		t.Fatalf("guarded recorders emitted signals: %+v", recorder)
	}

	source.recordRateLimits(target, 2)
	source.recordRedactions(target, 3)
	source.recordRetries(target, 4)
	if len(recorder.rateLimited) != 1 || recorder.rateLimited[0].count != 2 {
		t.Errorf("rateLimited = %+v, want one signal with count 2", recorder.rateLimited)
	}
	if len(recorder.redactions) != 1 || recorder.redactions[0].count != 3 {
		t.Errorf("redactions = %+v, want one signal with count 3", recorder.redactions)
	}
	if len(recorder.retries) != 1 || recorder.retries[0].count != 4 {
		t.Errorf("retries = %+v, want one signal with count 4", recorder.retries)
	}
}

func TestRecordFactsAggregatesCountsByKind(t *testing.T) {
	t.Parallel()

	recorder := &recordingMetrics{}
	source := metricsSource(recorder)
	source.recordFacts(metricsTarget(), []emittedFact{
		{Kind: KindObservedDashboard},
		{Kind: KindObservedDashboard},
		{Kind: KindObservedRule},
	})
	if len(recorder.facts) != 1 {
		t.Fatalf("len(facts) = %d, want 1", len(recorder.facts))
	}
	counts := recorder.facts[0].counts
	if counts[KindObservedDashboard] != 2 || counts[KindObservedRule] != 1 {
		t.Errorf("counts = %v, want dashboard 2 and rule 1", counts)
	}
	if recorder.facts[0].provider != ProviderGrafana {
		t.Errorf("provider = %q, want grafana", recorder.facts[0].provider)
	}
}
