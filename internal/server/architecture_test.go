package server

import "testing"

func TestArchitectureModules(t *testing.T) {
	if newDashboardRead(nil) == nil {
		t.Fatal("dashboard read module missing")
	}
	if newWorkflowEngine(nil) == nil {
		t.Fatal("workflow execution module missing")
	}
	if newCommandRouter(&Server{}) == nil {
		t.Fatal("command router missing")
	}
}
