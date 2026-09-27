package server

// commandOperation is a WebSocket command name. Keeping the wire values in one
// catalog makes the server's application protocol explicit without changing its
// JSON representation.
type commandOperation string

const (
	operationAppCreate           commandOperation = "app.create"
	operationAppTokenRotate      commandOperation = "app.token.rotate"
	operationAppTokenRevoke      commandOperation = "app.token.revoke"
	operationWorkflowPublish     commandOperation = "workflow.publish"
	operationCalendarSet         commandOperation = "calendar.set"
	operationCalendarNext        commandOperation = "calendar.next"
	operationCalendarOccurrences commandOperation = "calendar.occurrences"
	operationStartScheduleSet    commandOperation = "start-schedule.set"
	operationDashboardStats      commandOperation = "dashboard.stats"
	operationDashboardList       commandOperation = "dashboard.list"
	operationDeliveryReplay      commandOperation = "delivery.replay"
	operationDeliveryCancel      commandOperation = "delivery.cancel"

	operationScheduleSet      commandOperation = "schedule.set"
	operationScheduleCancel   commandOperation = "schedule.cancel"
	operationScheduleExtend   commandOperation = "schedule.extend"
	operationScheduleThrottle commandOperation = "schedule.throttle"
	operationDeliveryAck      commandOperation = "delivery.ack"
	operationWorkflowStart    commandOperation = "workflow.start"
	operationSignal           commandOperation = "signal"
)

type commandAudience uint8

const (
	adminCommand commandAudience = iota + 1
	applicationCommand
)

type commandSpec struct {
	audience            commandAudience
	requiresIdempotency bool
}

// classifyCommandOperation is the protocol seam between JSON frames and command
// routing. It deliberately excludes auth: auth is the WebSocket handshake, not
// a post-authentication command.
func classifyCommandOperation(op string) (commandSpec, bool) {
	switch commandOperation(op) {
	case operationAppCreate, operationAppTokenRotate, operationAppTokenRevoke,
		operationWorkflowPublish, operationCalendarSet, operationCalendarNext,
		operationCalendarOccurrences, operationStartScheduleSet, operationDashboardStats,
		operationDashboardList, operationDeliveryReplay, operationDeliveryCancel:
		return commandSpec{audience: adminCommand}, true
	case operationScheduleSet, operationScheduleCancel, operationScheduleExtend,
		operationScheduleThrottle, operationDeliveryAck, operationWorkflowStart,
		operationSignal:
		return commandSpec{audience: applicationCommand, requiresIdempotency: true}, true
	default:
		return commandSpec{}, false
	}
}

func validateAdminOperation(op string) string {
	spec, found := classifyCommandOperation(op)
	if !found || spec.audience != adminCommand {
		return "unknown admin op"
	}
	return ""
}

func validateApplicationOperation(op, idempotencyKey string) string {
	// Keep this order stable: callers have always received the idempotency error
	// before command validation.
	if idempotencyKey == "" {
		return "idempotencyKey required"
	}
	spec, found := classifyCommandOperation(op)
	if !found || spec.audience != applicationCommand {
		return "unknown op"
	}
	if spec.requiresIdempotency && idempotencyKey == "" {
		return "idempotencyKey required"
	}
	return ""
}
