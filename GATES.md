# Gates: Calcron admin dashboard

OWNS: web/**, internal/server/dashboard.go, internal/server/dashboard/**, internal/server/dashboard_test.go, Dockerfile, .dockerignore, .gitignore, README.md, scripts/check-dashboard.mjs

Scope: replace the inline HTML dashboard with a React/TypeScript admin UI built by Vite, embedded in the Go binary, covering all admin ops.

- [x] G0: this ledger states outcomes that can fail
  CHECK: node /home/byte/.agents/skills/unlazy/scripts/gate-lint.mjs GATES.md
  EXPECT: LINT OK
  EVIDENCE: automatic-evidence=v1; definition-sha256=699bb6fc7d38b5b5767622b575d66a138053384aa90bfd36d32a3d1044c6915e; exit=0; EXPECT=matched; output-sha256=48630b7361dd44ee870917b12c3d19b9d7bdea738aaca16bb04d4cab83b772d2; output-bytes=8; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=4de336199479/49 entries

- [x] G1: dashboard builds cleanly (typecheck + vite bundle)
  CHECK: npm run build && echo BUILD OK
  EXPECT: /BUILD OK/
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=e3a24e3c5be49c69fd5664b5874d926b914ab8bd498b28ff1ec2bbb372692e59; exit=0; EXPECT=matched; output-sha256=b059e51c9b383dfd123d1416cbb7cf03b6b618703b00fd35d2af20e07776866e; output-bytes=523; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=4de336199479/49 entries

- [x] G2: TypeScript reports no type errors
  CHECK: npm run typecheck && echo TYPECHECK OK
  EXPECT: /TYPECHECK OK/
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=6c5b6b155d60c2e4e334ed518f6a0792171090709cb25ae0312f52bb0b1b305d; exit=0; EXPECT=matched; output-sha256=6931c6de434636f167ffa7068200eba3318b72fa68998bc4c2c16e0e3d33bc27; output-bytes=65; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=4de336199479/49 entries

- [x] G3: embedded build is served at / with app root and hashed assets
  CHECK: go test ./internal/server/ -run TestDashboardServesEmbeddedBuild -count=1 -v && echo EMBED OK
  EXPECT: /EMBED OK/
  EVIDENCE: automatic-evidence=v1; definition-sha256=234eb23a3d81d736a1e879f849d02f999a90f714b469d042f0405946ae29b61b; exit=0; EXPECT=matched; output-sha256=53f473de172270e434f6c7fbc4c712e6a57e57a0d23256028b2844d64a4bae4c; output-bytes=163; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=4de336199479/49 entries

- [x] G4: server and SDK compile
  CHECK: go build ./... && echo GO BUILD OK
  EXPECT: /GO BUILD OK/
  EVIDENCE: automatic-evidence=v1; definition-sha256=b7d4f51ba65a67085668d84fb5b5407cc24badd59f35d687332f000dda692839; exit=0; EXPECT=matched; output-sha256=3ff7d628c170ee05dc421268f6bb0c3aa7258e1bbdfce25a25e2a198d60c3725; output-bytes=12; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=4de336199479/49 entries

- [x] G5: full Go suite still passes against a real PostgreSQL
  CHECK: go test ./... -count=1 && echo SUITE OK
  EXPECT: /SUITE OK/
  EVIDENCE: automatic-evidence=v1; definition-sha256=1edcc145f6e162f39669fa28c144fc94bb931c72474f965898046c91339e4652; exit=0; EXPECT=matched; output-sha256=950ff064dbc2531bc2f1a6e17a16d5fb08afe9690319db4bac2f1688c9695bc8; output-bytes=218; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=4de336199479/49 entries

- [x] G6: dashboard serves the real build, not the not-built placeholder
  CHECK: node scripts/check-dashboard.mjs
  EXPECT: dashboard build verified
  EVIDENCE: automatic-evidence=v1; definition-sha256=78e92bb30b20fd778db7d0ef7a3fa25fa4bccaedbf1443b3116c3de087e68357; exit=0; EXPECT=matched; output-sha256=7ebe44d3e3045bb2e3cdab0cd9030b4f66a9a81fcf75a85bb4ebb57e98cd45b2; output-bytes=165; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=4de336199479/49 entries

- [x] G7: dashboard unit checks pass (relative-time formatting)
  CHECK: npm test
  EXPECT: format verification passed
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=dd0d5b7fab88a2f0e7f962ef966e8791e078288823da17b9fe5513df69480f30; exit=0; EXPECT=matched; output-sha256=9e2925621833cd83de0d5b3247a581e493b68487b97386268f8d9ec65f4c8ad2; output-bytes=113; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=4de336199479/49 entries
