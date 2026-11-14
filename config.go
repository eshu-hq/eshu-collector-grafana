// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eshu-hq/eshu-collector-grafana/grafana"
)

// activationTargetJSON is the operator shape for one Grafana target.
// Secrets travel by env reference, never inline.
type activationTargetJSON struct {
	Provider         string   `json:"provider"`
	ScopeID          string   `json:"scope_id"`
	InstanceID       string   `json:"instance_id"`
	BaseURL          string   `json:"base_url"`
	TokenEnv         string   `json:"token_env"`
	ResourceLimit    int      `json:"resource_limit"`
	StaleAfter       string   `json:"stale_after"`
	Enabled          *bool    `json:"enabled"`
	DeclaredUIDs     []string `json:"declared_uids"`
	ObservedOnlyHint bool     `json:"observed_only_hint"`
}

// Settings is the validated per-claim configuration.
type Settings struct {
	Targets []grafana.TargetConfig
}

// ParseSettings validates the activation config for the claimed scope.
// getenv resolves credential environment variables the config names.
// Unknown keys fail closed so a misspelled limit cannot be silently
// ignored.
func ParseSettings(raw map[string]any, claimScopeID string, getenv func(string) string) (Settings, error) {
	var decoded struct {
		Targets []activationTargetJSON `json:"targets"`
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return Settings{}, configErrorf("grafana collector config is not JSON serializable")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return Settings{}, configErrorf("decode grafana collector config: %s", decodeReason(err))
	}
	if len(decoded.Targets) == 0 {
		return Settings{}, configErrorf("grafana collector requires at least one target")
	}
	targets := make([]grafana.TargetConfig, 0, len(decoded.Targets))
	seen := map[string]struct{}{}
	for i, item := range decoded.Targets {
		mapped, err := mapActivationTarget(item, getenv)
		if err != nil {
			return Settings{}, configErrorf("targets[%d]: %s", i, err.Error())
		}
		// Mirror NewClaimedSource: disabled targets are dropped before
		// client construction, so a disabled scope is never contacted and
		// its claim fails like an unconfigured scope.
		if !mapped.Enabled {
			continue
		}
		if _, dup := seen[mapped.ScopeID]; dup {
			return Settings{}, configErrorf("duplicate grafana target scope_id %q", mapped.ScopeID)
		}
		seen[mapped.ScopeID] = struct{}{}
		targets = append(targets, mapped)
	}
	if len(targets) == 0 {
		return Settings{}, configErrorf("grafana collector requires at least one enabled target")
	}
	if strings.TrimSpace(claimScopeID) != "" {
		found := false
		for _, target := range targets {
			if target.ScopeID == strings.TrimSpace(claimScopeID) {
				found = true
				break
			}
		}
		if !found {
			return Settings{}, configErrorf("claim scope %q is not configured", strings.TrimSpace(claimScopeID))
		}
	}
	return Settings{Targets: targets}, nil
}

func mapActivationTarget(item activationTargetJSON, getenv func(string) string) (grafana.TargetConfig, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	staleAfter, err := parseOptionalDuration(item.StaleAfter)
	if err != nil {
		return grafana.TargetConfig{}, err
	}
	enabled := true
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	declared := make(map[string]struct{}, len(item.DeclaredUIDs))
	for _, uid := range item.DeclaredUIDs {
		if trimmed := strings.TrimSpace(uid); trimmed != "" {
			declared[trimmed] = struct{}{}
		}
	}
	return grafana.TargetConfig{
		Provider:         strings.TrimSpace(item.Provider),
		ScopeID:          strings.TrimSpace(item.ScopeID),
		InstanceID:       strings.TrimSpace(item.InstanceID),
		BaseURL:          strings.TrimSpace(item.BaseURL),
		Token:            getenv(strings.TrimSpace(item.TokenEnv)),
		ResourceLimit:    item.ResourceLimit,
		StaleAfter:       staleAfter,
		Enabled:          enabled,
		DeclaredUIDs:     declared,
		ObservedOnlyHint: item.ObservedOnlyHint,
	}, nil
}

// parseOptionalDuration parses an optional duration string; empty means no
// freshness bound.
func parseOptionalDuration(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, configErrorf("invalid stale_after %q", raw)
	}
	return duration, nil
}

type configError struct{ message string }

func (e configError) Error() string { return e.message }

func configErrorf(format string, args ...any) configError {
	return configError{message: "invalid grafana activation config: " + fmt.Sprintf(format, args...)}
}
