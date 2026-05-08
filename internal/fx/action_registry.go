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

	if !in.Config.SelfHosted {
		reg.SetAllowlist(saasAllowedActions(reg.Names()))
	}

	return ActionRegistryOut{Registry: reg}
}

// saasAllowedActions filters out actions that should not be runnable on a
// hosted SaaS deployment (raw exec / js / file IO).
func saasAllowedActions(all []string) []string {
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
