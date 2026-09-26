# Durable delivery integration evidence

Source: user-requested release-readiness slice, derived from the durable-delivery testing decisions in `docs/specs/0001-calcron.md`.

| Guarantee | Test | Result |
| --- | --- | --- |
| Repeated schedule commands return the original reply and create one history entry. | `TestScheduleIdempotencyReturnsOriginalResult` | PASS |
| A disconnected application does not consume attempts and receives the same delivery ID after reconnecting. | `TestOfflineDeliveryDoesNotRetryAndReconnectsWithSameID` | PASS |
| Five unacknowledged sends advance attempts with exponential retry delays, then block; replay and cancellation retain identity and update state. | `TestBlockedDeliveryCanReplayAndCancel` | PASS |
| Rotating tokens overlaps access, revocation rejects the replacement token, and applications cannot run admin operations. | `TestTokenRevocationPreservesOverlappingTokenAndBlocksAdminOps` | PASS |
| One application cannot acknowledge another application's delivery. | `TestApplicationCannotAcknowledgeAnotherApplicationsDelivery` | PASS |

The test-first compile-time RED was two untyped JSON delivery IDs in the new test. Converting them at the dashboard boundary made the suite green; production code did not require a change.

Validation:

```sh
CALCRON_TEST_DATABASE_URL=... go test ./internal/server -run 'TestScheduleIdempotencyReturnsOriginalResult|TestOfflineDeliveryDoesNotRetryAndReconnectsWithSameID|TestBlockedDeliveryCanReplayAndCancel|TestTokenRevocationPreservesOverlappingTokenAndBlocksAdminOps|TestApplicationCannotAcknowledgeAnotherApplicationsDelivery' -count=1
# PASS

CALCRON_TEST_DATABASE_URL=... go test ./internal/server -cover -count=1
# coverage: 61.8% of statements
```

Coverage remains below the workflow's 80% target. Calendar recurrence, CEL branches, workflow-version pinning, and missed-occurrence policies are intentionally outside this delivery-focused slice.
