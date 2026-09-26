# At-least-once durable delivery

Calcron persists each due delivery until its application acknowledges it. If the application is disconnected, Calcron queues the delivery until it reconnects; if delivery or acknowledgement fails, Calcron retries it. Applications must make handlers idempotent by delivery ID or by the business object they change.

## Consequences

Calcron does not claim exactly-once effects across service boundaries. Duplicate events are valid and a delayed delivery may arrive after its original deadline.
