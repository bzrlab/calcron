# SDK integration TDD evidence

Source: application-SDK completion request, derived in this run.

Typed-contract extension: event names now carry their payload type through the
Node schema and Go generic event declarations.

| Guarantee | Test | Result |
| --- | --- | --- |
| Node client sends every application command, including throttle, and returns typed replies | `sdk/node/index.test.ts` | PASS |
| Node client rejects bad credentials, exposes delivery acknowledgement, protocol errors, out-of-order replies, duplicate deliveries, and reconnects | `sdk/node/index.test.ts` | PASS |
| Go client sends every application command, validates deadline input, serializes chains, decodes typed results, acknowledges delivery, reconnects, and rejects bad credentials | `sdk/go/client_test.go` | PASS |
| The Node package exposes an ESM build and declarations; the guide covers the public contract | `npm pack --dry-run`, `scripts/check-sdk-docs.mjs` | PASS |
| Typed Node schemas reject unknown, mismatched, and missing event/workflow payloads at compile time | `sdk/node/typecheck.test.ts` | PASS |
| Typed Go declarations decode deliveries and serialize schedules with their declared payload type | `sdk/go/client_test.go` | PASS |

## RED → GREEN

The Node contract test first failed because `client.throttle` did not exist.
The Go contract test first failed because `Set` returned only an error and
`Throttle` did not exist. After adding feature parity and typed response
contracts, the focused Node and Go suites passed.

## Coverage

- Node: 100% lines, 93.62% branches, 96.67% functions (`node --experimental-strip-types --experimental-test-coverage --test index.test.ts`).
- Go: 82.6% statements (`go test ./sdk/go -count=1 -cover`).

The focused tests use an in-process protocol double for Node and a real local
WebSocket server for Go. Full Calcron deployment behavior remains covered by
the repository integration suite.
