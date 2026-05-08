package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/parser"
)

func validateCmd(noColorFlag *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <workflow.yaml>",
		Short: "Validate a workflow file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)

			return executeValidate(args[0], noColor)
		},
	}
}

func executeValidate(path string, noColor bool) error {
	r := &cliRenderer{noColor: noColor}

	wf, err := parser.Parse(path)
	if err != nil {
		fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("31", "Validation failed: "+err.Error()))
		os.Exit(1)
	}

	r.wfName = wf.Name

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)

	for _, step := range wf.Steps {
		if !reg.Has(step.Action) {
			fmt.Printf("  %s %s\n", r.c("31", "✗"),
				r.c("31", fmt.Sprintf("Validation failed: step %q references unknown action %q", step.ID, step.Action)))
			os.Exit(1)
		}
	}

	buildErr := r.buildTree(wf)
	if buildErr != nil {
		fmt.Printf("  %s %s\n", r.c("31", "✗"), r.c("31", "Validation failed: "+buildErr.Error()))
		os.Exit(1)
	}

	fmt.Printf("  %s %s\n", r.c("32", "✓"),
		r.c("32", fmt.Sprintf("Workflow %q is valid (%d steps, %d params)", wf.Name, len(wf.Steps), len(wf.Params))))

	r.printTree()

	return nil
}
