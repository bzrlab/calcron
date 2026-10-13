// Package workflow owns declarative workflow validation.
package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
)

type Definition struct {
	Initial string           `json:"initial"`
	States  map[string]State `json:"states"`
}
type State struct {
	Type            string          `json:"type"`
	After           string          `json:"after"`
	Event           string          `json:"event"`
	Correlation     string          `json:"correlationKey"`
	CorrelationExpr string          `json:"correlationKeyExpr"`
	When            string          `json:"when"`
	True            string          `json:"true"`
	False           string          `json:"false"`
	Next            string          `json:"next"`
	Target          string          `json:"target"`
	Data            json.RawMessage `json:"data"`
	DataExpr        string          `json:"dataExpr"`
}

func Parse(raw json.RawMessage) (Definition, error) {
	var definition Definition
	if json.Unmarshal(raw, &definition) != nil || definition.Initial == "" {
		return definition, errors.New("invalid workflow definition")
	}
	initial, ok := definition.States[definition.Initial]
	if !ok || initial.Type == "" {
		return definition, errors.New("workflow initial state not found")
	}
	for name, state := range definition.States {
		if name == "" {
			return definition, errors.New("empty workflow state")
		}
		switch state.Type {
		case "end":
		case "wait_time":
			if _, err := time.ParseDuration(state.After); err != nil || state.Next == "" {
				return definition, errors.New("invalid wait_time")
			}
			if _, ok := definition.States[state.Next]; !ok {
				return definition, errors.New("workflow target not found")
			}
		case "wait_signal":
			if state.Event == "" || (state.Correlation == "") == (state.CorrelationExpr == "") || state.Next == "" {
				return definition, errors.New("invalid wait_signal")
			}
			if state.CorrelationExpr != "" {
				if _, err := CompileCEL(state.CorrelationExpr); err != nil {
					return definition, err
				}
			}
			if _, ok := definition.States[state.Next]; !ok {
				return definition, errors.New("workflow target not found")
			}
		case "branch":
			if state.When == "" || state.True == "" || state.False == "" {
				return definition, errors.New("invalid branch")
			}
			if _, err := CompileCEL(state.When); err != nil {
				return definition, err
			}
			if _, ok := definition.States[state.True]; !ok {
				return definition, errors.New("workflow target not found")
			}
			if _, ok := definition.States[state.False]; !ok {
				return definition, errors.New("workflow target not found")
			}
		case "emit":
			if state.Target == "" || state.Event == "" || state.Next == "" {
				return definition, errors.New("invalid emit")
			}
			if state.DataExpr != "" {
				if _, err := CompileCEL(state.DataExpr); err != nil {
					return definition, err
				}
			}
			if _, ok := definition.States[state.Next]; !ok {
				return definition, errors.New("workflow target not found")
			}
		default:
			return definition, errors.New("unknown workflow state")
		}
	}
	return definition, nil
}

const celCostLimit uint64 = 10000

// A nil checker.Cost estimator panics when the expression reads a dyn field.
type celSizeUnknown struct{}

func (celSizeUnknown) EstimateSize(checker.AstNode) *checker.SizeEstimate { return nil }
func (celSizeUnknown) EstimateCallCost(string, string, *checker.AstNode, []checker.AstNode) *checker.CallEstimate {
	return nil
}

func CompileCEL(expr string) (cel.Program, error) {
	env, err := cel.NewEnv(cel.Variable("input", cel.DynType), cel.Variable("state", cel.DynType))
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	estimate, err := checker.Cost(ast.NativeRep(), celSizeUnknown{})
	if err != nil {
		return nil, err
	}
	if estimate.Min > celCostLimit {
		return nil, fmt.Errorf("CEL cost %d exceeds limit %d", estimate.Min, celCostLimit)
	}
	return env.Program(ast, cel.CostLimit(celCostLimit), cel.InterruptCheckFrequency(100))
}
