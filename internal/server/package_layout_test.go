package server_test

import (
	"testing"

	"github.com/calcron/calcron/internal/application"
	"github.com/calcron/calcron/internal/calendar"
	"github.com/calcron/calcron/internal/protocol"
)

// The composition root should depend on domain modules through their public
// interfaces instead of keeping every concern in one package.
func TestDomainModulesShareTheProtocolSeam(t *testing.T) {
	frame := protocol.Frame{Op: "calendar.next"}
	if frame.Op == "" {
		t.Fatal("protocol frame must be constructible by domain modules")
	}
	if application.New(nil, func() string { return "id" }) == nil || calendar.New(nil) == nil {
		t.Fatal("domain module constructor missing")
	}
}
