package fx

import (
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
	uberfx "go.uber.org/fx"
)

type WorkflowIn struct {
	uberfx.In

	Config Config
}

type WorkflowOut struct {
	uberfx.Out

	Workflow *parser.Workflow
}

func NewWorkflow(in WorkflowIn) (out WorkflowOut, err error) {
	wf, parseErr := parser.Parse(in.Config.WorkflowPath)
	if parseErr != nil {
		return out, fmt.Errorf("parse workflow: %w", parseErr)
	}

	out.Workflow = wf

	return out, nil
}
