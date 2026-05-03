// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package assets

import (
	"strings"
	"testing"
)

func TestEmbeddedCockpitHTML_Present(t *testing.T) {
	data, err := FS.ReadFile("cockpit.html")
	if err != nil {
		t.Fatalf("read cockpit.html: %v", err)
	}
	if len(data) < 50 {
		t.Errorf("cockpit.html too small: %d bytes", len(data))
	}
}

func TestEmbeddedCockpitHTML_HasV4Markers(t *testing.T) {
	data, err := FS.ReadFile("cockpit.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	mustContain := []string{
		"--bg-base:",                        // token system
		"--accent-500:",                     // token system
		"cv4-pulse-halo",                    // halo animation
		"cv4-fade-up",                       // fade-up animation
		"prefers-reduced-motion",            // a11y
		"role=\"tablist\"",                  // ARIA
		"id=\"olm-graph\"",                  // SVG container the JS targets
		"window.addEventListener('message'", // postMessage handler
	}
	for _, m := range mustContain {
		if !strings.Contains(body, m) {
			t.Errorf("cockpit.html missing required marker: %q", m)
		}
	}
	// Size budget: < 100 KB (per spec risk #4).
	if len(data) > 100*1024 {
		t.Errorf("cockpit.html size %d bytes exceeds 100 KB budget", len(data))
	}
}
