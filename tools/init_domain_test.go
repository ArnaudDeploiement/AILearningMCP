// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna/tutor-mcp
// SPDX-License-Identifier: MIT

package tools

import (
	"strings"
	"testing"
)

func TestInitDomain_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "", "init_domain", map[string]any{
		"name":          "math",
		"concepts":      []string{"a"},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestInitDomain_MissingName(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          "",
		"concepts":      []string{"a"},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "name is required") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestInitDomain_NameTooLong(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          strings.Repeat("x", maxDomainNameLen+1),
		"concepts":      []string{"a"},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "name too long") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestInitDomain_NoConcepts(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          "math",
		"concepts":      []string{},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "at least one concept") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestInitDomain_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)

	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          "math",
		"concepts":      []string{"derivative", "integral"},
		"prerequisites": map[string][]string{"integral": {"derivative"}},
		"personal_goal": "do real analysis",
		"value_framings": map[string]any{
			"financial":    "make more money",
			"intellectual": "think clearly",
		},
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["domain_id"] == nil {
		t.Fatalf("expected domain_id in response, got %v", out)
	}

	// Domain saved
	d, err := store.GetDomainByLearner("L_owner")
	if err != nil {
		t.Fatalf("domain not saved: %v", err)
	}
	if d.Name != "math" {
		t.Fatalf("name not saved, got %q", d.Name)
	}
	if d.PersonalGoal != "do real analysis" {
		t.Fatalf("goal not saved, got %q", d.PersonalGoal)
	}
	if !strings.Contains(d.ValueFramingsJSON, "make more money") {
		t.Fatalf("value framings not persisted: %q", d.ValueFramingsJSON)
	}

	// ConceptStates initialised
	for _, c := range []string{"derivative", "integral"} {
		cs, err := store.GetConceptState("L_owner", c)
		if err != nil || cs == nil {
			t.Fatalf("concept state for %q missing: %v", c, err)
		}
	}
}

func TestInitDomain_PersonalGoalTooLong(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          "math",
		"concepts":      []string{"a"},
		"prerequisites": map[string][]string{},
		"personal_goal": strings.Repeat("x", maxPersonalGoalLen+1),
	})
	if !res.IsError || !strings.Contains(resultText(res), "personal_goal too long") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestInitDomain_InvalidConcepts(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerInitDomain, "L_owner", "init_domain", map[string]any{
		"name":          "math",
		"concepts":      []string{"a", ""},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "empty concept name") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestAddConcepts_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerAddConcepts, "L_owner", "add_concepts", map[string]any{
		"domain_id":     d.ID,
		"concepts":      []string{"c", "d"},
		"prerequisites": map[string][]string{"c": {"a"}, "d": {"c"}},
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["added"].(float64) != 2 {
		t.Fatalf("expected added=2, got %v", out["added"])
	}

	got, _ := store.GetDomainByID(d.ID)
	expected := []string{"a", "b", "c", "d"}
	if len(got.Graph.Concepts) != len(expected) {
		t.Fatalf("expected %d concepts, got %d (%v)", len(expected), len(got.Graph.Concepts), got.Graph.Concepts)
	}
	if got.Graph.Prerequisites["c"][0] != "a" || got.Graph.Prerequisites["d"][0] != "c" {
		t.Fatalf("prerequisites not merged: %+v", got.Graph.Prerequisites)
	}
}

func TestAddConcepts_DuplicateNotReadded(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerAddConcepts, "L_owner", "add_concepts", map[string]any{
		"domain_id":     d.ID,
		"concepts":      []string{"a"},
		"prerequisites": map[string][]string{},
	})
	if res.IsError {
		t.Fatalf("expected success, got %q", resultText(res))
	}
	out := decodeResult(t, res)
	if out["added"].(float64) != 0 {
		t.Fatalf("expected added=0 for duplicate, got %v", out["added"])
	}
}

func TestAddConcepts_NoConcepts(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerAddConcepts, "L_owner", "add_concepts", map[string]any{
		"domain_id":     "x",
		"concepts":      []string{},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "at least one concept") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestAddConcepts_DomainNotFound(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerAddConcepts, "L_owner", "add_concepts", map[string]any{
		"domain_id":     "missing",
		"concepts":      []string{"x"},
		"prerequisites": map[string][]string{},
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}
