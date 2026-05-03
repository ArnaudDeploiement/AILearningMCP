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
		"--cream-100:",                      // cream token namespace
		"--border-subtle:",                  // border token namespace
		"cv4-pulse-halo",                    // halo animation
		"cv4-fade-up",                       // fade-up animation
		"prefers-reduced-motion",            // a11y
		"role=\"tablist\"",                  // ARIA
		"id=\"olm-graph\"",                  // SVG container the JS targets
		"window.addEventListener('message'", // postMessage handler
		"ui/initialize",                     // MCP Apps handshake: View → Host request
		"ui/notifications/initialized",      // MCP Apps handshake: View → Host notification
		"ui/notifications/tool-result",      // MCP Apps inbound: Host → View tool result
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

func TestEmbeddedCockpitHTML_HasTab2Markers(t *testing.T) {
	data, err := FS.ReadFile("cockpit.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	mustContain := []string{
		"id=\"tab-global\"",   // Tab 2 trigger (already present, but assert it's still present after refactor)
		"id=\"panel-global\"", // Tab 2 panel
		"renderGlobal",        // JS function
		"calibration_history", // structuredContent key referenced by JS
		"recent_events",       // structuredContent key referenced by JS
	}
	for _, m := range mustContain {
		if !strings.Contains(body, m) {
			t.Errorf("cockpit.html missing required marker: %q", m)
		}
	}
}

func TestEmbeddedCockpitHTML_HasResponsiveAndDomainMarkers(t *testing.T) {
	data, err := FS.ReadFile("cockpit.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	mustContain := []string{
		"id=\"domain-select\"",                 // domain selector UI
		"id=\"fullscreen-toggle\"",             // fullscreen toggle button
		"available_domains",                    // OLMGraph payload key
		"ui/request-display-mode",              // View → Host request
		"ui/notifications/size-changed",        // View → Host size notification
		"ui/notifications/host-context-changed", // Host → View context updates
		"availableDisplayModes",                // capability advertised in ui/initialize
		"\"fullscreen\"",                       // declared mode
	}
	for _, m := range mustContain {
		if !strings.Contains(body, m) {
			t.Errorf("cockpit.html missing required marker: %q", m)
		}
	}
}
