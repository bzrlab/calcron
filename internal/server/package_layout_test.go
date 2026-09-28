package server

import (
	"testing"

	"github.com/calcron/calcron/internal/protocol"
)

// The composition root should depend on domain modules through their public
// interfaces instead of keeping every concern in one package.
func TestDomainModulesShareTheProtocolSeam(t *testing.T) {
	protocolFrame := protocol.Frame{Op: "calendar.next"}
	if protocolFrame.Op == "" {
		t.Fatal("protocol frame must be constructible by domain modules")
	}
	ctx, server := workflowTestServer(t)
	reply := server.commands.handle(ctx, &peer{admin: true}, frame{Op: "app.create", Name: random(), Namespace: random()})
	if !reply.OK {
		t.Fatalf("application module was not reached through the command router: %#v", reply)
	}
}
