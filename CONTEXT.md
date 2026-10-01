# Calcron

Calcron is a shared lifecycle scheduler for applications. It durably holds future work and delivers it when due.

## Language

**Application**:
A client system registered with Calcron that creates schedules and receives deliveries.
_Avoid_: Service, consumer, client

**Schedule**:
One named future delivery with a deadline, event name, payload, and lifecycle state.
_Avoid_: Cron job, timer, task

**Workflow definition**:
A versioned declarative description of states, transitions, waits, and effects that Calcron can run without application-specific code.
_Avoid_: App logic, script

**Workflow version**:
An immutable published revision of a workflow definition. Each workflow instance remains pinned to one version.
_Avoid_: Live workflow edit

**Workflow instance**:
One durable run of a workflow definition with its own input, state, and history.
_Avoid_: Job, session

**Workflow state**:
One named point in a workflow definition where an instance waits, branches, emits a command, or ends.
_Avoid_: Step, node

**Signal**:
An application event correlated to a workflow instance that may advance its state.
_Avoid_: Callback, response

**Correlation key**:
An application-defined value that identifies which waiting workflow instance may consume a signal.
_Avoid_: Lookup field, matching rule

**Idempotency key**:
An application-supplied value that identifies one state-changing command or signal. Repeating it returns the original result without changing state again.
_Avoid_: Request ID, retry token

**Command**:
A durable delivery from a workflow to an application that asks it to perform an outside effect.
_Avoid_: Business completion, action result

**Condition**:
A read-only CEL expression over workflow input, state, and signals that selects a workflow transition or derives a value.
_Avoid_: Script, business code

**Business calendar**:
A named timezone-aware set of eligible and ineligible dates used by workflows to calculate valid schedule times.
_Avoid_: Weekend setting, cron calendar

**Start schedule**:
A named calendar-aware recurrence that starts one workflow instance at each eligible occurrence.
_Avoid_: Cron job, repeating timer

**Broken start schedule**:
A start schedule that can never compute another occurrence and has been retired from firing. It keeps its identity and history, and returns to active when an application re-saves it.
_Avoid_: Failed job, disabled timer

**Due batch**:
One bounded claim of ready work, taken by whichever replica gets there first. A batch row must either be completed or returned to the queue; anything left eligible but unprocessable starves the batch.
_Avoid_: Batch job, poll

**Starved**:
A due row that a claim keeps selecting but no replica can process, so it occupies batch capacity indefinitely. Delivery claims skip applications with no connected peer; start schedule claims break or defer rows they cannot resolve.
_Avoid_: Backlog, stuck job

**Missed occurrence policy**:
A start schedule's explicit instruction to skip, run one late instance, or create every missed instance after downtime.
_Avoid_: Recovery default, catch-up behavior

**Schedule key**:
An application-scoped, stable name for one schedule. Reusing it changes that schedule rather than creating another.
_Avoid_: Timer ID, job ID

**Lifecycle policy**:
A generic rule that determines how a schedule changes after a new request or a delivery result.
_Avoid_: Smart logic, business rule

**Chain**:
A lifecycle policy that creates a named successor schedule only after the parent delivery is acknowledged.
_Avoid_: Follow-up timer, workflow

**Extension**:
A lifecycle change that adds a duration to a schedule's existing deadline. For a due schedule, it adds the duration from the current time.
_Avoid_: Reset, postpone

**Throttle**:
A lifecycle policy that permits one immediate delivery for a key, then ignores triggers for a defined window.
_Avoid_: Debounce, rate limit

**Delivery**:
One durable attempt to send a due schedule's event to its application.
_Avoid_: Execution, callback
