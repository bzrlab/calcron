# Gates: calendar view gaps

OWNS: internal/server/**, web/**, README.md, .cursor/rules/protocol-and-sdk.mdc, GATES.md

Scope: close four calendar gaps: range projection without a per-Start-schedule cap, Start schedule runs in the Calendars editor, a display timezone choice, and drag-and-drop to move a Start schedule run.

- [x] G0: this ledger states outcomes that can fail
  CHECK: node /home/byte/.agents/skills/unlazy/scripts/gate-lint.mjs GATES.md
  EXPECT: LINT OK
  EVIDENCE: automatic-evidence=v1; definition-sha256=699bb6fc7d38b5b5767622b575d66a138053384aa90bfd36d32a3d1044c6915e; exit=0; EXPECT=matched; output-sha256=07de1a3fdbb119780824944843dcfc7da4b1349f87411f5d2df255b3caa7c614; output-bytes=150; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=b7656ec2eed0/49 entries

- [x] G1: calendar.occurrences returns every run in a range, for a named or inline calendar, and rejects oversize ranges
  CHECK: go test ./internal/server/ -run TestCalendarOccurrences -count=1 -v
  EXPECT: /--- PASS: TestCalendarOccurrences/
  EVIDENCE: automatic-evidence=v1; definition-sha256=0d4fd1c4339c1bb8f936b29b8255cfff0609e5a2123f627d31138c69deccf179; exit=0; EXPECT=matched; output-sha256=e15d0ca635ea7581629f744689fc0684df09c0939ae23c90ded38a3f8986117d; output-bytes=136; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=b7656ec2eed0/49 entries

- [x] G2: timezone helpers map instants to a zone's day and minute and back, across DST
  CHECK: node --experimental-strip-types src/lib/calendar.test.ts
  EXPECT: calendar verification passed
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=e2f28b91c9e40c7dc4d62edefb5e400d3fcb50deb62ba4e4d1db08e39dd3e0fb; exit=0; EXPECT=matched; output-sha256=2a63515cdd3624adee8f54c3271b0f258538855733ac0c7cce93e0a54049dec5; output-bytes=29; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=b7656ec2eed0/49 entries

- [x] G3: TypeScript reports no type errors
  CHECK: npm run typecheck && echo TYPECHECK OK
  EXPECT: /TYPECHECK OK/
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=6c5b6b155d60c2e4e334ed518f6a0792171090709cb25ae0312f52bb0b1b305d; exit=0; EXPECT=matched; output-sha256=6931c6de434636f167ffa7068200eba3318b72fa68998bc4c2c16e0e3d33bc27; output-bytes=65; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=b7656ec2eed0/49 entries

- [x] G4: all web unit checks pass
  CHECK: npm test
  EXPECT: calendar verification passed
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=70f56dda801dd1b15cf9e6825941569157ce318588b246dfa350f8e2339bc176; exit=0; EXPECT=matched; output-sha256=1fa221691047f34a59898806947cabf0e3e8f95be13d5106786233609d17bb1e; output-bytes=293; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=b7656ec2eed0/49 entries

- [x] G5: dashboard bundle builds and is embedded
  CHECK: npm run build && node ../scripts/check-dashboard.mjs
  EXPECT: dashboard build verified
  CWD: web
  EVIDENCE: automatic-evidence=v1; definition-sha256=8af02985784cd52c0725fa40ab9f557fba5c033a58aca14ac3a11d321421c6e0; exit=0; EXPECT=matched; output-sha256=cadfe0fe04ff38850de7aac4d9c5f6cd359084e0a1b36ff94e4d3f0595ed4309; output-bytes=679; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron/web; path=b7656ec2eed0/49 entries

- [x] G6: full Go suite passes against a real PostgreSQL
  CHECK: go vet ./... && go test ./... -count=1 && echo SUITE OK
  EXPECT: /SUITE OK/
  EVIDENCE: automatic-evidence=v1; definition-sha256=3462b4cf9a09925e84087cdd58699a6b998932ce0051d2175757e8b1ed4cb51a; exit=0; EXPECT=matched; output-sha256=51502b50fbf9f08b1f02dae7b901c4a8f7a89ba110afdb59b35e04b7b53b0396; output-bytes=218; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=b7656ec2eed0/49 entries

- [x] G7: the admin op list names calendar.occurrences in README and the protocol rule
  CHECK: node -e "const f=require('fs');const ok=['README.md','.cursor/rules/protocol-and-sdk.mdc'].every(p=>f.readFileSync(p,'utf8').includes('calendar.occurrences'));console.log(ok?'OPS DOCUMENTED':'MISSING');process.exit(ok?0:1)"
  EXPECT: OPS DOCUMENTED
  EVIDENCE: automatic-evidence=v1; definition-sha256=9895095f2118fdff3438a95f084134cf47069a78e0462f6c43509224f1dbafdf; exit=0; EXPECT=matched; output-sha256=28f1716ec77d10849ea87a741662056e12ed23fe5409890a586245fd7037340a; output-bytes=15; shell=/bin/sh; cwd=/mnt/Development/Projects/calcron; path=b7656ec2eed0/49 entries

- [x] G8: in the browser, dragging a Start schedule run in week view moves its local time, the editor grid shows draft-projected runs, and the timezone picker moves chips
  EVIDENCE: drop at 10:00 in week view showed preview "10:00", confirm dialog, then start_schedules row read local_time=10:00 input={"region":"north"}; display zone UTC/America/New_York/Asia/Dhaka put chips at 04:00/12:00 AM/10:00 on the same days; editor closing Tue Sep 29 removed its run, opening Fri Oct 2 added one, Discard restored both, NextDays followed the draft; after review fixes: drop on another day column refused (no preview), own-day drop previewed 11:00, drawer times carry the display zone (GMT+6), Enter on a chip inside an editor cell left the day unchanged while Enter on the cell toggled it
