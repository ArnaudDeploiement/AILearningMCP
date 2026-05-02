// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestRegisterTools_Smoke wires every tool through RegisterTools and lists them
// over an MCP session. This exercises the full registration code path and
// guards against accidental signature drift between handlers and the registry.
func TestRegisterTools_Smoke(t *testing.T) {
	_, deps := setupToolsTest(t)

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	RegisterTools(server, deps)

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := []string{
		"get_pending_alerts",
		"get_next_activity",
		"record_interaction",
		"check_mastery",
		"get_learner_context",
		"get_availability_model",
		"get_cockpit_state",
		"init_domain",
		"add_concepts",
		"update_learner_profile",
		"record_affect",
		"calibration_check",
		"record_calibration_result",
		"get_autonomy_metrics",
		"get_metacognitive_mirror",
		"feynman_challenge",
		"transfer_challenge",
		"record_transfer_result",
		"learning_negotiation",
		"archive_domain",
		"unarchive_domain",
		"delete_domain",
		"get_misconceptions",
		"record_session_close",
		"queue_webhook_message",
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	var missing []string
	for _, w := range want {
		if !got[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing registered tools: %s", strings.Join(missing, ","))
	}
}
