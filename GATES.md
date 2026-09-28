# Gates: server package tree

OWNS: internal/server/**, internal/schedule/**, internal/delivery/**, internal/workflow/**, internal/dashboard/**, internal/idempotency/**, GATES.md

Scope: Split the remaining server-owned operational logic into real domain packages while preserving protocol behavior.

- [ ] G1: Server composition depends on domain modules instead of owning schedule, delivery, workflow, dashboard, and idempotency implementations.
  CHECK: go test ./internal/server -run TestPackageLayout -count=1
  EXPECT: ok  	github.com/bzrlab/calcron/internal/server
  EVIDENCE: pending

- [ ] G2: Refactored Go packages compile and keep existing behavior.
  CHECK: go test ./...
  EXPECT: ok  	github.com/bzrlab/calcron/internal/server
  EVIDENCE: pending

- [ ] G3: Domain modules are not cosmetic wrappers; they own production Go source outside internal/server.
  EVIDENCE: pending manual review of internal/{schedule,delivery,workflow,dashboard,idempotency}
