// Package workflow owns declarative workflow validation.
package workflow

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/cel-go/cel"
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
	if json.Unmarshal(raw, &definition) != nil || definition.Initial == "" || definition.States[definition.Initial].Type == "" {
		return definition, errors.New("invalid workflow definition")
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

func CompileCEL(expr string) (cel.Program, error) {
	env, err := cel.NewEnv(cel.Variable("input", cel.DynType), cel.Variable("state", cel.DynType))
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expr)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	return env.Program(ast)
}
