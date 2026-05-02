// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna/tutor-mcp
// SPDX-License-Identifier: MIT

package auth

import (
	"sync"
	"testing"
)

// resetTrustedProxies fully resets the sync.Once + slice so loadTrustedProxies
// re-reads the environment. Tests using this MUST run sequentially because
// the package-level state is shared.
func resetTrustedProxies(t *testing.T) {
	t.Helper()
	trustedProxiesOnce = sync.Once{}
	trustedProxies = nil
}

func TestLoadTrustedProxies_FreshParseValidEntries(t *testing.T) {
	resetTrustedProxies(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8, 192.168.0.0/16, ,fc00::/7")
	t.Cleanup(func() { resetTrustedProxies(t) })

	got := loadTrustedProxies()
	if len(got) != 3 {
		t.Fatalf("expected 3 valid CIDRs (one entry blank skipped), got %d: %v", len(got), got)
	}
	// Sanity: trustedProxies global must be the same backing slice.
	if &got[0] != &trustedProxies[0] {
		t.Fatal("loadTrustedProxies must return the package-level slice")
	}
}

func TestLoadTrustedProxies_FreshParseSkipsInvalidEntries(t *testing.T) {
	resetTrustedProxies(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "not-a-cidr, 10.0.0.0/8, also/garbage, 172.16.0.0/12")
	t.Cleanup(func() { resetTrustedProxies(t) })

	got := loadTrustedProxies()
	if len(got) != 2 {
		t.Fatalf("expected 2 valid CIDRs (invalid skipped), got %d: %v", len(got), got)
	}
}

func TestLoadTrustedProxies_FreshEmptyEnvProducesNil(t *testing.T) {
	resetTrustedProxies(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	t.Cleanup(func() { resetTrustedProxies(t) })

	got := loadTrustedProxies()
	if got != nil {
		t.Fatalf("expected nil when env unset, got %v", got)
	}
}

func TestLoadTrustedProxies_OnceIsIdempotent(t *testing.T) {
	resetTrustedProxies(t)
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	t.Cleanup(func() { resetTrustedProxies(t) })

	first := loadTrustedProxies()
	if len(first) != 1 {
		t.Fatalf("first call: len=%d, want 1", len(first))
	}
	// Now change the env — Once.Do already ran; second call must still return
	// the original parsed slice (not re-parse).
	t.Setenv("TRUSTED_PROXY_CIDRS", "192.168.0.0/16")
	second := loadTrustedProxies()
	if len(second) != 1 {
		t.Fatalf("second call: len=%d, want 1 (cached)", len(second))
	}
	if &first[0] != &second[0] {
		t.Fatal("loadTrustedProxies must return identical slice header on repeat")
	}
}
