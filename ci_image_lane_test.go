// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

package collector

import (
	"os"
	"strings"
	"testing"
)

// TestImageLaneBuildsOnImageInputs pins the PR-time image lane: the changes
// job must classify Dockerfile and .dockerignore as the image class, the
// misc catch-all must negate both so an image-only PR runs the image lane
// and nothing else, and the image job must build the shipped Dockerfile
// with a digest-pin proof instead of pushing.
func TestImageLaneBuildsOnImageInputs(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(".github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"image: ${{ steps.filter.outputs.image }}",
		"- 'Dockerfile'",
		"- '.dockerignore'",
		"- '!Dockerfile'",
		"- '!.dockerignore'",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("ci.yml lacks %q: an image-only PR would fall through to misc", want)
		}
	}
	for _, marker := range []string{
		"docker build",
		"digest-pinned",
		"65532:65532",
		"Dockerfile bases",
	} {
		if !strings.Contains(text, marker) {
			t.Errorf("ci.yml image lane lacks %q: the image proof is gone", marker)
		}
	}
	if !strings.Contains(text, "gosec-sarif, image]") {
		t.Errorf("ci-complete does not wait on the image lane: an image break could merge green")
	}
}
