# Calcron Go SDK

Import `github.com/calcron/calcron/sdk/go` (its package name is `cron`) to
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

Read the [SDK guide](https://github.com/calcron/calcron/blob/main/docs/sdk.md)
for every operation, delivery and idempotency rules, and the complete example.
