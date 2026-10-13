package workflow

import (
	"strings"
	"testing"
)

func TestCompileCELRejectsStaticCostOverLimit(t *testing.T) {
	if _, err := CompileCEL("true"); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCEL(`'k:' + input.missing`); err != nil {
		t.Fatal(err)
	}
	var expr strings.Builder
	expr.WriteString("1")
	for i := 0; i < 20000; i++ {
		expr.WriteString("+1")
	}
	if _, err := CompileCEL(expr.String()); err == nil {
		t.Fatal("accepted expression over the CEL cost limit")
	}
}
