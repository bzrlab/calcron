package server

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	calcron "github.com/bzrlab/calcron"
)

func TestParseWorkflowRejectsMissingTransitionTarget(t *testing.T) {
	definition := map[string]any{
		"initial": "wait",
		"states": map[string]any{
			"wait": map[string]any{"type": "wait_time", "after": "1s", "next": "missing"},
		},
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseWorkflow(raw); err == nil {
		t.Fatal("missing transition target accepted")
	}
}

func TestWorkflowVersionPinsRunningInstance(t *testing.T) {
	ctx, s := workflowTestServer(t)
	app := workflowTestApp(t, ctx, s)
	definition := func(trueState, falseState string) json.RawMessage {
		return workflowJSON(t, map[string]any{
			"initial": "wait",
			"states": map[string]any{
				"wait":     map[string]any{"type": "wait_signal", "event": "approved", "correlationKey": "order-1", "next": "branch"},
				"branch":   map[string]any{"type": "branch", "when": "input.approved == true", "true": trueState, "false": falseState},
				"approved": map[string]any{"type": "end"},
				"rejected": map[string]any{"type": "end"},
			},
		})
	}
	if got := s.publishWorkflow(ctx, frame{ApplicationID: app, Name: "approval", Data: definition("approved", "rejected")}); !got.OK {
		t.Fatalf("publish v1: %#v", got)
	}
	started := s.startWorkflow(ctx, app, frame{Name: "approval", Data: json.RawMessage(`{"approved":true}`), IdempotencyKey: "start-v1"})
	if !started.OK {
		t.Fatalf("start v1: %#v", started)
	}
	id := started.Data.(map[string]string)["instanceId"]
	if got := s.publishWorkflow(ctx, frame{ApplicationID: app, Name: "approval", Data: definition("rejected", "approved")}); !got.OK {
		t.Fatalf("publish v2: %#v", got)
	}
	if got := s.signal(ctx, app, frame{Event: "approved", Correlation: "order-1", Data: json.RawMessage(`{}`), IdempotencyKey: "signal-v1"}); !got.OK {
		t.Fatalf("signal: %#v", got)
	}
	var version int
	var state, status string
	if err := s.db.QueryRow(ctx, `select workflow_version,current_state,status from workflow_instances where id=$1`, id).Scan(&version, &state, &status); err != nil {
		t.Fatal(err)
	}
	if version != 1 || state != "approved" || status != "completed" {
		t.Fatalf("instance used replacement definition: version=%d state=%q status=%q", version, state, status)
	}
}

func TestWorkflowCELEvaluatesBothBranches(t *testing.T) {
	ctx, s := workflowTestServer(t)
	app := workflowTestApp(t, ctx, s)
	definition := workflowJSON(t, map[string]any{
		"initial": "branch",
		"states": map[string]any{
			"branch":   map[string]any{"type": "branch", "when": "input.approved == true", "true": "approved", "false": "rejected"},
			"approved": map[string]any{"type": "end"},
			"rejected": map[string]any{"type": "end"},
		},
	})
	if got := s.publishWorkflow(ctx, frame{ApplicationID: app, Name: "approval", Data: definition}); !got.OK {
		t.Fatalf("publish: %#v", got)
	}
	for _, tc := range []struct {
		name, input, want string
	}{
		{"true", `{"approved":true}`, "approved"},
		{"false", `{"approved":false}`, "rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := s.startWorkflow(ctx, app, frame{Name: "approval", Data: json.RawMessage(tc.input), IdempotencyKey: tc.name})
			if !started.OK {
				t.Fatalf("start: %#v", started)
			}
			var state, status string
			if err := s.db.QueryRow(ctx, `select current_state,status from workflow_instances where id=$1`, started.Data.(map[string]string)["instanceId"]).Scan(&state, &status); err != nil {
				t.Fatal(err)
			}
			if state != tc.want || status != "completed" {
				t.Fatalf("branch: state=%q status=%q", state, status)
			}
		})
	}
}

func TestCELRejectsInvalidAndRestrictedExpressions(t *testing.T) {
	for _, expression := range []string{"input.approved =", "now() == 1"} {
		definition := workflowJSON(t, map[string]any{
			"initial": "branch",
			"states": map[string]any{
				"branch": map[string]any{"type": "branch", "when": expression, "true": "yes", "false": "no"},
				"yes":    map[string]any{"type": "end"},
				"no":     map[string]any{"type": "end"},
			},
		})
		if _, err := parseWorkflow(definition); err == nil {
			t.Fatalf("accepted %q", expression)
		}
	}
}

func workflowTestServer(t *testing.T) (context.Context, *Server) {
	t.Helper()
	db := os.Getenv("CALCRON_TEST_DATABASE_URL")
	if db == "" {
		t.Skip("CALCRON_TEST_DATABASE_URL required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := New(ctx, Config{DatabaseURL: db, AdminToken: "test-admin", Migration: calcron.InitialMigration})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return ctx, s
}

func workflowTestApp(t *testing.T, ctx context.Context, s *Server) string {
	t.Helper()
	created := s.applications.Create(ctx, frame{Name: random(), Namespace: random()})
	if !created.OK {
		t.Fatalf("create app: %#v", created)
	}
	return created.Data.(map[string]string)["applicationId"]
}

func workflowJSON(t *testing.T, definition map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
