package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tailflow/tailflow/internal/action"
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/internal/runtime"
)

func testCmd(noColorFlag *bool) *cobra.Command {
	var (
		caseName string
		listFlag bool
	)

	cmd := &cobra.Command{
		Use:   "test <workflow.yaml>",
		Short: "Run workflow test cases",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noColor := detectNoColor(*noColorFlag)

			if listFlag {
				return executeTestList(args[0], noColor)
			}

			return executeTest(args[0], caseName, noColor)
		},
	}

	cmd.Flags().StringVar(&caseName, "case", "", "Run a specific test case")
	cmd.Flags().BoolVar(&listFlag, "list", false, "List all test cases")

	return cmd
}

func collectTestCases(wf *parser.Workflow) []string {
	seen := make(map[string]bool)
	var names []string

	for _, step := range wf.Steps {
		for _, tc := range step.Testing {
			if !seen[tc.Name] {
				seen[tc.Name] = true
				names = append(names, tc.Name)
			}
		}
	}

	return names
}

func executeTestList(path string, noColor bool) error {
	r := &cliRenderer{noColor: noColor}

	wf, err := parser.Parse(path)
	if err != nil {
		return err
	}

	cases := collectTestCases(wf)
	if len(cases) == 0 {
		fmt.Printf("  %s No test cases found in %q\n", r.c("33", "⚠"), wf.Name)

		return nil
	}

	fmt.Printf("\n  Test cases for %q:\n\n", wf.Name)

	var headerBuilder strings.Builder

	fmt.Fprintf(&headerBuilder, "  %-20s", "")

	for _, step := range wf.Steps {
		fmt.Fprintf(&headerBuilder, "%-18s", step.ID)
	}

	fmt.Println(r.c("1", headerBuilder.String()))

	for _, caseName := range cases {
		var rowBuilder strings.Builder

		fmt.Fprintf(&rowBuilder, "  %-20s", caseName)

		for _, step := range wf.Steps {
			rowBuilder.WriteString(testCaseCell(r, step, caseName))
		}

		fmt.Println(rowBuilder.String())
	}

	fmt.Println()

	return nil
}

func colorPad(r *cliRenderer, code, text string) string {
	padded := fmt.Sprintf("%-18s", text)
	if r.noColor {
		return padded
	}

	return fmt.Sprintf("\033[%sm%s\033[0m", code, padded)
}

func testCaseCell(r *cliRenderer, step parser.Step, caseName string) string {
	tc := findTestCaseInStep(step, caseName)

	switch {
	case tc == nil:
		return colorPad(r, "90", "(runs)")
	case tc.Error != nil:
		return colorPad(r, "31", "mock error")
	case tc.Output != nil && tc.Expect != nil:
		return colorPad(r, "36", "mock+expect")
	case tc.Output != nil:
		return colorPad(r, "33", "mock")
	case tc.Expect != nil:
		return colorPad(r, "36", "expect")
	default:
		return colorPad(r, "90", "(runs)")
	}
}

func findTestCaseInStep(step parser.Step, caseName string) *parser.TestCase {
	for i := range step.Testing {
		if step.Testing[i].Name == caseName {
			return &step.Testing[i]
		}
	}

	return nil
}

func executeTest(path string, caseName string, noColor bool) error {
	wf, err := parser.Parse(path)
	if err != nil {
		return err
	}

	cases := []string{caseName}
	if caseName == "" {
		cases = collectTestCases(wf)
		if len(cases) == 0 {
			r := &cliRenderer{noColor: noColor}
			fmt.Printf("  %s No test cases found in %q\n", r.c("33", "⚠"), wf.Name)

			return nil
		}
	}

	r := &cliRenderer{noColor: noColor}

	fmt.Printf("\n  Testing %q...\n\n", wf.Name)

	passed := 0
	failed := 0

	for _, cn := range cases {
		start := time.Now()
		testErr := runSingleTestCase(wf, cn)
		elapsed := time.Since(start).Round(time.Millisecond)

		if testErr != nil {
			fmt.Printf("  %s  %-20s %s %s\n", r.c("31", "✗"), cn, r.c("90", fmt.Sprintf("(%s)", elapsed)), r.c("31", testErr.Error()))

			failed++
		} else {
			fmt.Printf("  %s  %-20s %s\n", r.c("32", "✓"), cn, r.c("90", fmt.Sprintf("passed (%s)", elapsed)))

			passed++
		}
	}

	fmt.Printf("\n  %d/%d passed\n\n", passed, len(cases))

	if failed > 0 {
		os.Exit(1)
	}

	return nil
}

func runSingleTestCase(wf *parser.Workflow, caseName string) error {
	bus := event.NewBus()
	defer bus.Close()

	reg := action.NewRegistry()
	action.RegisterBuiltins(reg)
	reg.SetAllowlist(cliAllowedActions(reg.Names()))

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	exec := engine.NewExecutor(reg, bus, logger, wf.Sensitive, nil, nil)

	services := &runtime.ActionServices{
		DBPool:     runtime.NewMemoryDBPool(),
		TxRegistry: runtime.NewMemoryTxRegistry(logger),
		Locker:     runtime.NewMemoryLocker(),
		KVStore:    runtime.NewMemoryKVStore(),
	}

	ctx := context.Background()

	result, err := exec.Execute(ctx, wf, nil, engine.ExecuteOptions{
		Services:     services,
		TestCaseName: caseName,
	})
	if err != nil {
		return err
	}

	if result.Status == runtime.StatusFailed {
		if result.Error != nil {
			return result.Error
		}

		return errors.New("workflow failed")
	}

	return nil
}
