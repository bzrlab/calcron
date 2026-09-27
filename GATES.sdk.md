# Gates: Calcron application SDKs

OWNS: sdk/node/**, sdk/go/**, docs/sdk.md, docs/sdk-integration.tdd.md, README.md, scripts/check-sdk-docs.mjs, GATES.sdk.md, .gitignore

Scope: ship documented, package-ready Node and Go application SDKs with complete application-command parity and verified protocol behavior.

- [x] G0: this ledger states outcomes that can fail
  CHECK: node /home/byte/.agents/skills/unlazy/scripts/gate-lint.mjs GATES.sdk.md
  EXPECT: LINT OK
  EVIDENCE: automatic-evidence=v1; definition-sha256=cc6761aea1091544846f344bdd4b4be5474b44cfa41846214aad34e2fce9d151; exit=0; EXPECT=matched; output-sha256=48630b7361dd44ee870917b12c3d19b9d7bdea738aaca16bb04d4cab83b772d2; output-bytes=8; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=e6f1e170844d/48 entries

- [x] G1: Node SDK builds, typechecks, and exercises its public protocol client
  CHECK: npm --prefix sdk/node test
  EXPECT: node sdk verification passed
  EVIDENCE: automatic-evidence=v1; definition-sha256=6b29e89e3c554266a72828e95ed5bf524d2c9cfd0e02adabe8a6f951431d761d; exit=0; EXPECT=matched; output-sha256=fc784efd8a96076292b2d02accc358db17a7236964b9e8cba6ca4cd71cf8d4a5; output-bytes=1669; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=e6f1e170844d/48 entries

- [x] G2: Go SDK exercises its public protocol client
  CHECK: GOCACHE=/tmp/calcron-go-build go test ./sdk/go -count=1
  EXPECT: github.com/calcron/calcron/sdk/go
  EVIDENCE: automatic-evidence=v1; definition-sha256=77052c0982e7386c0693a8775d39da994693ab0a7809718b2ad751e0039960e5; exit=0; EXPECT=matched; output-sha256=8bdb9ad44ea15ca5887c3bc7821221783f53ecfd9df67ba8366a8a07d9099ccf; output-bytes=46; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=e6f1e170844d/48 entries

- [x] G3: SDK documentation covers installation, all application operations, durable delivery, idempotency, and both language examples
  CHECK: node scripts/check-sdk-docs.mjs
  EXPECT: sdk documentation verified
  EVIDENCE: automatic-evidence=v1; definition-sha256=8a59bd5f7b1f6c547fca488bf9d30e5a36ca2995610068b58197b5f06be52892; exit=0; EXPECT=matched; output-sha256=579c3bc0a440da5536b276e972440cc9d3c5fd10e1f3d6d31e5a6b40b6afc5ac; output-bytes=27; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=e6f1e170844d/48 entries

- [x] G4: SDK changes preserve the repository test suite
  CHECK: GOCACHE=/tmp/calcron-go-build go test ./... -count=1
  EXPECT: github.com/calcron/calcron/sdk/go
  EVIDENCE: automatic-evidence=v1; definition-sha256=a299a5b4d14595d7431d7ae130ccd639793da992d8c421726cab9abbb2722111; exit=0; EXPECT=matched; output-sha256=d844160cc4f036dcdfde9e5dfd247d29d371372b72c171069d75631105185db3; output-bytes=209; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=e6f1e170844d/48 entries
