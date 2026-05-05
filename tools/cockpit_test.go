// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"tutor-mcp/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetCockpitState_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "", "get_cockpit_state", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}

func TestGetCockpitState_NoDomain(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{})
	if !res.IsError || !strings.Contains(resultText(res), "aucun domaine configuré") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestGetCockpitState_ForeignDomainRejected(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")
	res := callTool(t, deps, registerGetCockpitState, "L_attacker", "get_cockpit_state", map[string]any{
		"domain_id": d.ID,
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestGetCockpitState_HappyPath(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	// Initialize a couple of concept states with mastery.
	cs1 := models.NewConceptState("L_owner", "a")
	cs1.PMastery = 0.95
	_ = store.InsertConceptStateIfNotExists(cs1)
	_ = store.UpsertConceptState(cs1)

	cs2 := models.NewConceptState("L_owner", "b")
	cs2.PMastery = 0.4
	_ = store.InsertConceptStateIfNotExists(cs2)
	_ = store.UpsertConceptState(cs2)

	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)

	if _, ok := out["domains"]; !ok {
		t.Fatalf("expected domains key, got %v", out)
	}
	if _, ok := out["alerts"]; !ok {
		t.Fatalf("expected alerts key, got %v", out)
	}
	if _, ok := out["autonomy_score"]; !ok {
		t.Fatalf("expected autonomy_score, got %v", out)
	}
	if _, ok := out["global_progress"]; !ok {
		t.Fatalf("expected global_progress, got %v", out)
	}
}

func TestGetCockpitState_AllDomains(t *testing.T) {
	store, deps := setupToolsTest(t)
	makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{})
	if res.IsError {
		t.Fatalf("got %q", resultText(res))
	}
	out := decodeResult(t, res)
	domains, ok := out["domains"].([]any)
	if !ok || len(domains) != 1 {
		t.Fatalf("expected 1 domain in cockpit, got %v", out["domains"])
	}
}

func TestGetCockpitState_UnknownDomainID(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerGetCockpitState, "L_owner", "get_cockpit_state", map[string]any{
		"domain_id": "nope",
	})
	if !res.IsError || !strings.Contains(resultText(res), "domain not found") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestCockpitResource_Registered(t *testing.T) {
	_, deps := setupToolsTest(t)
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registerCockpitResource(srv, deps)

	c, s := mcp.NewInMemoryTransports()
	go srv.Run(t.Context(), s)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(t.Context(), c, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: cockpitResourceURI})
	if err != nil {
		t.Fatalf("ReadResource ui://cockpit: %v", err)
	}
	if len(res.Contents) == 0 {
		t.Fatal("no contents returned")
	}
	body := res.Contents[0].Text
	if !strings.Contains(body, "<html") && !strings.Contains(body, "<!DOCTYPE") {
		preview := body
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Errorf("expected HTML content, got: %q", preview)
	}
	const wantMIME = "text/html;profile=mcp-app"
	if res.Contents[0].MIMEType != wantMIME {
		t.Errorf("MIMEType=%q, want %q", res.Contents[0].MIMEType, wantMIME)
	}
}

func TestOpenCockpit_ReturnsMetaAndStructuredContent(t *testing.T) {
	store, deps := setupToolsTest(t)
	d := makeOwnerDomain(t, store, "L_owner", "math")

	res := callTool(t, deps, registerOpenCockpit, "L_owner", "open_cockpit", map[string]any{
		"domain_id": d.ID,
	})
	if res.IsError {
		t.Fatalf("got error: %q", resultText(res))
	}
	// _meta.ui.resourceUri must be set
	uiMeta, ok := res.Meta["ui"].(map[string]any)
	if !ok {
		t.Fatalf("_meta.ui missing or wrong type: %+v", res.Meta)
	}
	if uiMeta["resourceUri"] != cockpitResourceURI {
		t.Errorf("_meta.ui.resourceUri=%v, want %s", uiMeta["resourceUri"], cockpitResourceURI)
	}
	// structuredContent must marshal to OLMGraph shape — read directly off res.StructuredContent.
	scBytes, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structuredContent: %v", err)
	}
	var sc map[string]any
	if err := json.Unmarshal(scBytes, &sc); err != nil {
		t.Fatalf("unmarshal structuredContent: %v", err)
	}
	if sc["domain_id"] != d.ID {
		t.Errorf("structuredContent.domain_id=%v, want %s", sc["domain_id"], d.ID)
	}
	if _, ok := sc["concepts"]; !ok {
		t.Errorf("structuredContent.concepts missing: %+v", sc)
	}
	if _, ok := sc["streak"]; !ok {
		t.Errorf("structuredContent.streak missing: %+v", sc)
	}
	// content[0].text must be non-empty fallback (FormatOLMEmbed.Description text)
	if len(res.Content) == 0 {
		t.Fatal("Content empty — text fallback missing")
	}
	if txt := resultText(res); txt == "" {
		t.Errorf("Content[0].Text empty — fallback prose should be non-empty for any seeded domain")
	}
}

func TestOpenCockpit_NoAuth(t *testing.T) {
	_, deps := setupToolsTest(t)
	res := callTool(t, deps, registerOpenCockpit, "", "open_cockpit", map[string]any{})
	if !res.IsError {
		t.Fatalf("expected auth error")
	}
}
