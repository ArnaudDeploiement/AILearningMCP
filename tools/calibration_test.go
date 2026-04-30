package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"learning-runtime/auth"
	"learning-runtime/db"
	"learning-runtime/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

var calibTestCounter int

func setupCalibTest(t *testing.T) (*db.Store, *Deps) {
	t.Helper()
	calibTestCounter++
	dsn := fmt.Sprintf("file:calibmem_%s_%d?mode=memory&cache=shared", t.Name(), calibTestCounter)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(raw); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, id := range []string{"L_owner", "L_attacker"} {
		_, err := raw.Exec(
			`INSERT INTO learners (id, email, password_hash, objective, created_at) VALUES (?, ?, 'hash', 'test', ?)`,
			id, id+"@test.com", now,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { raw.Close() })
	store := db.NewStore(raw)
	deps := &Deps{
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return store, deps
}

// callRecordCalibration registers the tool, connects an in-memory MCP session
// with a receiving middleware that injects the given learner ID into ctx, then
// invokes record_calibration_result.
func callRecordCalibration(t *testing.T, deps *Deps, learnerID, predictionID string, actual float64) (*mcp.CallToolResult, error) {
	t.Helper()
	ctx := context.Background()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerRecordCalibrationResult(server, deps)
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			ctx = context.WithValue(ctx, auth.LearnerIDKey, learnerID)
			return next(ctx, method, req)
		}
	})

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	argsJSON, _ := json.Marshal(map[string]any{
		"prediction_id": predictionID,
		"actual_score":  actual,
	})
	return session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "record_calibration_result",
		Arguments: json.RawMessage(argsJSON),
	})
}

func TestRecordCalibrationResult_OwnerAllowed(t *testing.T) {
	store, deps := setupCalibTest(t)

	rec := &models.CalibrationRecord{
		PredictionID: "cal_owner_1",
		LearnerID:    "L_owner",
		ConceptID:    "Concept_A",
		Predicted:    0.75,
	}
	if err := store.CreateCalibrationPrediction(rec); err != nil {
		t.Fatal(err)
	}

	res, err := callRecordCalibration(t, deps, "L_owner", "cal_owner_1", 0.80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		var text string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		t.Fatalf("owner write should succeed, got error: %s", text)
	}

	// Verify the record was actually updated.
	saved, err := store.GetCalibrationRecord("cal_owner_1")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if saved.Actual == nil || *saved.Actual != 0.80 {
		t.Fatalf("expected actual=0.80, got %+v", saved.Actual)
	}
}

func TestRecordCalibrationResult_ForeignLearnerRejected(t *testing.T) {
	store, deps := setupCalibTest(t)

	rec := &models.CalibrationRecord{
		PredictionID: "cal_victim_1",
		LearnerID:    "L_owner",
		ConceptID:    "Concept_A",
		Predicted:    0.30,
	}
	if err := store.CreateCalibrationPrediction(rec); err != nil {
		t.Fatal(err)
	}

	res, err := callRecordCalibration(t, deps, "L_attacker", "cal_victim_1", 0.99)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error result, got success")
	}
	var text string
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "not found") {
		t.Fatalf("expected neutral 'not found' message, got %q", text)
	}

	// Verify the record was NOT modified.
	saved, err := store.GetCalibrationRecord("cal_victim_1")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if saved.Actual != nil {
		t.Fatalf("record should remain untouched, got actual=%v", *saved.Actual)
	}
	if saved.LearnerID != "L_owner" {
		t.Fatalf("owner should remain L_owner, got %q", saved.LearnerID)
	}
}
