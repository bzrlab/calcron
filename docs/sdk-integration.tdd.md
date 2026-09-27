# SDK integration TDD evidence

Source: application-SDK completion request, derived in this run.

| Guarantee | Test | Result |
| --- | --- | --- |
| Node client sends every application command, including throttle, and returns typed replies | `sdk/node/index.test.ts` | PASS |
| Node client rejects bad credentials, exposes delivery acknowledgement, protocol errors, and reconnects | `sdk/node/index.test.ts` | PASS |
| Go client sends every application command, decodes typed results, acknowledges delivery, reconnects, and rejects bad credentials | `sdk/go/client_test.go` | PASS |
| The Node package exposes an ESM build and declarations; the guide covers the public contract | `npm pack --dry-run`, `scripts/check-sdk-docs.mjs` | PASS |

## RED → GREEN

The Node contract test first failed because `client.throttle` did not exist.
The Go contract test first failed because `Set` returned only an error and
`Throttle` did not exist. After adding feature parity and typed response
contracts, the focused Node and Go suites passed.

## Coverage

- Node: 100% lines, 92.5% branches, 94.12% functions (`node --experimental-strip-types --experimental-test-coverage --test index.test.ts`).
- Go: 82.4% statements (`go test ./sdk/go -count=1 -cover`).

The focused tests use an in-process protocol double for Node and a real local
WebSocket server for Go. Full Calcron deployment behavior remains covered by
the repository integration suite.
