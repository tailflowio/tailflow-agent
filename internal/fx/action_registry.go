package fx

import (
	"github.com/tailflow/tailflow/internal/action"
	uberfx "go.uber.org/fx"
)

type ActionRegistryIn struct {
	uberfx.In

	Config Config
}

type ActionRegistryOut struct {
	uberfx.Out

	Registry *action.Registry
}

func NewActionRegistry(in ActionRegistryIn) ActionRegistryOut {
	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	if !in.Config.Unsafe {
		reg.SetAllowlist(safeAllowedActions(reg.Names()))
	}

	return ActionRegistryOut{Registry: reg}
}

// safeAllowedActions filters out actions that are unsafe to expose by default
// (raw exec / js / file IO).
func safeAllowedActions(all []string) []string {
	blocked := map[string]bool{
		"js":         true,
		"exec":       true,
		"file.read":  true,
		"file.write": true,
	}

	allowed := make([]string, 0, len(all))

	for _, name := range all {
		if blocked[name] {
			continue
		}

		allowed = append(allowed, name)
	}

	return allowed
}
