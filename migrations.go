package calcron

import _ "embed"

//go:embed db/migrations/0001_init.sql
var InitialMigration string
