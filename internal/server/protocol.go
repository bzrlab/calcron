package server

import "github.com/bzrlab/calcron/internal/protocol"

// Aliases preserve the internal server test surface while protocol is the
// dependency-safe seam for domain modules.
type frame = protocol.Frame
type reply = protocol.Reply

func fail(err string) reply { return reply{Error: err} }
func ok(data any) reply     { return reply{OK: true, Data: data} }
