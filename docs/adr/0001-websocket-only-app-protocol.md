# WebSocket-only application protocol

Calcron is private infrastructure for its owner's applications. V1 uses one authenticated WebSocket endpoint for schedule commands and due-event delivery, because owned applications can maintain connections and durable PostgreSQL-backed delivery removes the need for separate inbound webhook endpoints.

## Consequences

The Node and Go SDKs own reconnects, heartbeats, request correlation, and acknowledgement. HTTP webhooks and a REST application API are outside V1 and can be added only for applications that cannot keep a connection. Issue #5 is the recorded exception: HTTP commands are not refused while a socket is open.
