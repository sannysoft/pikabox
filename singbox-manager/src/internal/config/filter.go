package config

import (
	"fmt"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// FilterEnv is what a subscription filter expression sees for each candidate:
// name/type/server/port shortcuts plus the full sing-box outbound object, e.g.
//
//	type == "vless" && name matches "(?i)nl|us" && outbound.tls.reality.enabled == true
type FilterEnv struct {
	Name     string         `expr:"name"`
	Type     string         `expr:"type"`
	Server   string         `expr:"server"`
	Port     int            `expr:"port"`
	Outbound map[string]any `expr:"outbound"`
}

// CompileFilter compiles a subscription filter; an empty filter keeps all.
func CompileFilter(source string) error {
	_, err := compileFilter(source)
	return err
}

func compileFilter(source string) (*vm.Program, error) {
	if strings.TrimSpace(source) == "" {
		return nil, nil
	}
	return expr.Compile(source, expr.Env(FilterEnv{}), expr.AsBool())
}

// Filter returns a predicate for a subscription filter expression. Candidates
// on which the expression errors at runtime (e.g. a missing nested field) are
// rejected rather than aborting the whole subscription.
func Filter(source string) (func(FilterEnv) bool, error) {
	program, err := compileFilter(source)
	if err != nil {
		return nil, fmt.Errorf("filter: %w", err)
	}
	if program == nil {
		return func(FilterEnv) bool { return true }, nil
	}
	return func(env FilterEnv) bool {
		out, err := expr.Run(program, env)
		keep, _ := out.(bool)
		return err == nil && keep
	}, nil
}
