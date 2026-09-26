package server

import (
	"encoding/json"
	"testing"
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
