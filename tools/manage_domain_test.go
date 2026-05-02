// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"

	"tutor-mcp/models"
)

func makeOwnerDomain(t *testing.T, store interface {
	CreateDomainWithValueFramings(string, string, string, models.KnowledgeSpace, string) (*models.Domain, error)
}, ownerID, name string) *models.Domain {
	t.Helper()
	d, err := store.CreateDomainWithValueFramings(ownerID, name, "", models.KnowledgeSpace{
		Concepts:      []string{"a", "b"},
		Prerequisites: map[string][]string{"b": {"a"}},
	}, "")
	if err != nil {
		t.Fatalf("create domain: %v", err)
	}
	return d
}

func TestArchiveDomain_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerArchiveDomain, "", "archive_domain", map[string]any{"domain_id": "d_x"})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
	if !strings.Contains(resultText(res), "authentication") {
		t.Fatalf("expected authentication required, got %q", resultText(res))
	}
}

func TestArchiveDomain_MissingID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerArchiveDomain, "L_owner", "archive_domain", map[string]any{
		"domain_id": "",
	})
	if !res.IsError {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(resultText(res), "domain_id is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestArchiveDomain_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerArchiveDomain, "L_owner", "archive_domain", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}

	// DB state: domain is archived
	got, err := store.GetDomainByID(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived {
		t.Fatalf("expected archived=true after archive_domain")
	}
}

func TestArchiveDomain_ForeignDomainRejected(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerArchiveDomain, "L_attacker", "archive_domain", map[string]any{
		"domain_id": d.ID,
	})
	if !res.IsError {
		t.Fatalf("expected error result for foreign learner")
	}
	if !strings.Contains(resultText(res), "not found") {
		t.Fatalf("expected 'not found', got %q", resultText(res))
	}

	// DB state: should remain unarchived
	got, _ := store.GetDomainByID(d.ID)
	if got.Archived {
		t.Fatalf("foreign archive should not have modified the domain")
	}
}

func TestArchiveDomain_UnknownID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerArchiveDomain, "L_owner", "archive_domain", map[string]any{
		"domain_id": "nope",
	})
	if !res.IsError {
		t.Fatalf("expected error")
	}
	if !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestUnarchiveDomain_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	if err := store.ArchiveDomain(d.ID, "L_owner"); err != nil {
		t.Fatal(err)
	}

	res := callTool(t, deps, registerUnarchiveDomain, "L_owner", "unarchive_domain", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}

	got, _ := store.GetDomainByID(d.ID)
	if got.Archived {
		t.Fatalf("expected unarchived")
	}
}

func TestUnarchiveDomain_MissingID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerUnarchiveDomain, "L_owner", "unarchive_domain", map[string]any{
		"domain_id": "",
	})
	if !res.IsError {
		t.Fatalf("expected validation error")
	}
}

func TestUnarchiveDomain_ForeignRejected(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	_ = store.ArchiveDomain(d.ID, "L_owner")

	res := callTool(t, deps, registerUnarchiveDomain, "L_attacker", "unarchive_domain", map[string]any{
		"domain_id": d.ID,
	})
	if !res.IsError {
		t.Fatalf("expected error for foreign learner")
	}
}

func TestDeleteDomain_RequiresConfirm(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerDeleteDomain, "L_owner", "delete_domain", map[string]any{
		"domain_id": d.ID,
		"confirm":   false,
	})
	if !res.IsError {
		t.Fatalf("expected error when confirm=false")
	}
	if !strings.Contains(resultText(res), "confirm must be true") {
		t.Fatalf("got %q", resultText(res))
	}

	// domain still exists
	got, err := store.GetDomainByID(d.ID)
	if err != nil || got == nil {
		t.Fatalf("domain should still exist after unconfirmed delete")
	}
}

func TestDeleteDomain_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerDeleteDomain, "L_owner", "delete_domain", map[string]any{
		"domain_id": d.ID,
		"confirm":   true,
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}

	// domain is gone
	if _, err := store.GetDomainByID(d.ID); err == nil {
		t.Fatalf("expected domain deleted")
	}
}

func TestDeleteDomain_ForeignRejected(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerDeleteDomain, "L_attacker", "delete_domain", map[string]any{
		"domain_id": d.ID,
		"confirm":   true,
	})
	if !res.IsError {
		t.Fatalf("expected error for foreign learner")
	}
	// Domain still exists.
	if _, err := store.GetDomainByID(d.ID); err != nil {
		t.Fatalf("expected domain preserved, got err %v", err)
	}
}

func TestDeleteDomain_MissingID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerDeleteDomain, "L_owner", "delete_domain", map[string]any{
		"domain_id": "",
		"confirm":   true,
	})
	if !res.IsError {
		t.Fatalf("expected validation error")
	}
}
