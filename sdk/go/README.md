# Calcron Go SDK

Import `github.com/bzrlab/calcron/sdk/go` (its package name is `cron`) to
connect a Go application to Calcron. The client authenticates over WebSocket,
reconnects after a transport loss, correlates replies, and exposes durable
delivery acknowledgements.

```go
client, err := cron.Connect(ctx, os.Getenv("CALCRON_URL"), os.Getenv("CALCRON_TOKEN"))
if err != nil { log.Fatal(err) }
defer client.Close()

type InvoiceDue struct { InvoiceID string `json:"invoiceId"` }
var InvoiceDueEvent = cron.EventOf[InvoiceDue]("invoice.due")
cron.OnTyped(client, InvoiceDueEvent, func(event cron.TypedDelivery[InvoiceDue]) {
	if err := persistReceipt(event.ID, event.Data); err != nil { return }
	_, _ = event.Ack(context.Background())
})
```

Read the [SDK guide](https://github.com/bzrlab/calcron/blob/main/docs/sdk.md)
for every operation, delivery and idempotency rules, and the complete example.

## Immediate delivery

Use `client.Enqueue(ctx, cron.Enqueue{Event: "invoice.due", Data: payload, IdempotencyKey: "invoice:42:deliver:v1"})`, or `cron.EnqueueTyped(ctx, client, cron.TypedEnqueue[Payload]{Event: event, Data: payload, IdempotencyKey: key})` for a typed event.

Returns `scheduleId` and `runAt` (`ScheduleResult` in Go). The idempotency key is optional; omitting it creates a new immediate schedule on every invocation. Supply a stable key for safe retries. Delivery uses the usual retry and acknowledgement rules. See [the command guide](https://github.com/bzrlab/calcron/blob/main/docs/enqueue.md).

## Connection concurrency

Use `cron.Connect(ctx, url, token, cron.ConnectOptions{MaxInFlight: 4})` to cap
distinct unacknowledged deliveries on this connection. Omitted or zero means
unlimited. The cap is retained on reconnect; retries reuse a slot and still
require idempotent handlers.
See [delivery cap semantics](https://github.com/bzrlab/calcron/blob/main/docs/concurrency.md).
