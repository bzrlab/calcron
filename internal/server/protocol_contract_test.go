package server

import (
	"context"
	"testing"
)

func TestCommandOperationContract(t *testing.T) {
	tests := []struct {
		name                string
		op                  commandOperation
		audience            commandAudience
		requiresIdempotency bool
	}{
		{"app create", operationAppCreate, adminCommand, false},
		{"token rotate", operationAppTokenRotate, adminCommand, false},
		{"token revoke", operationAppTokenRevoke, adminCommand, false},
		{"workflow publish", operationWorkflowPublish, adminCommand, false},
		{"calendar set", operationCalendarSet, adminCommand, false},
		{"calendar next", operationCalendarNext, adminCommand, false},
		{"calendar occurrences", operationCalendarOccurrences, adminCommand, false},
		{"start schedule set", operationStartScheduleSet, adminCommand, false},
		{"dashboard stats", operationDashboardStats, adminCommand, false},
		{"dashboard list", operationDashboardList, adminCommand, false},
		{"delivery replay", operationDeliveryReplay, adminCommand, false},
		{"delivery cancel", operationDeliveryCancel, adminCommand, false},
		{"schedule set", operationScheduleSet, applicationCommand, true},
		{"schedule cancel", operationScheduleCancel, applicationCommand, true},
		{"schedule extend", operationScheduleExtend, applicationCommand, true},
		{"schedule throttle", operationScheduleThrottle, applicationCommand, true},
		{"delivery ack", operationDeliveryAck, applicationCommand, true},
		{"workflow start", operationWorkflowStart, applicationCommand, true},
		{"signal", operationSignal, applicationCommand, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec, found := classifyCommandOperation(string(test.op))
			if !found {
				t.Fatalf("operation %q is not classified", test.op)
			}
			if spec.audience != test.audience || spec.requiresIdempotency != test.requiresIdempotency {
				t.Fatalf("classification for %q = %#v", test.op, spec)
			}
		})
	}
}

func TestCommandOperationValidationPreservesRoutingRules(t *testing.T) {
	if got := validateAdminOperation(string(operationScheduleSet)); got != "unknown admin op" {
		t.Fatalf("admin application operation error = %q", got)
	}
	if got := validateAdminOperation("unknown"); got != "unknown admin op" {
		t.Fatalf("admin unknown operation error = %q", got)
	}
	if got := validateApplicationOperation(string(operationAppCreate), "key"); got != "unknown op" {
		t.Fatalf("application admin operation error = %q", got)
	}
	if got := validateApplicationOperation("unknown", "key"); got != "unknown op" {
		t.Fatalf("application unknown operation error = %q", got)
	}
	if got := validateApplicationOperation("unknown", ""); got != "idempotencyKey required" {
		t.Fatalf("application validation order changed: %q", got)
	}
	if got := validateApplicationOperation(string(operationScheduleThrottle), "key"); got != "" {
		t.Fatalf("application operation rejected: %q", got)
	}
}

func TestCommandRouterRejectsWrongAudienceBeforeDispatch(t *testing.T) {
	router := commandRouter{}
	if got := router.handle(context.Background(), &peer{admin: true}, frame{Op: string(operationScheduleSet)}); got.Error != "unknown admin op" {
		t.Fatalf("admin router error = %q", got.Error)
	}
	if got := router.handle(context.Background(), &peer{}, frame{Op: string(operationAppCreate)}); got.Error != "idempotencyKey required" {
		t.Fatalf("application router error = %q", got.Error)
	}
}
