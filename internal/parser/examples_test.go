package parser_test

import (
	"path/filepath"
	"testing"

	"github.com/tailflow/tailflow/internal/parser"
)

// TestExamplesAreValid parses every workflow under the repository examples
// directory and fails if any of them is rejected. It guards against shipping
// example workflows that do not satisfy the v2 schema (a regression that would
// make the documented `tailflow serve examples/...` commands fail).
func TestExamplesAreValid(t *testing.T) {
	paths, globErr := filepath.Glob("../../examples/*.yaml")
	if globErr != nil {
		t.Fatalf("glob examples: %v", globErr)
	}

	if len(paths) == 0 {
		t.Fatal("no example workflows found under ../../examples")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, parseErr := parser.Parse(path)
			if parseErr != nil {
				t.Errorf("example %s is invalid: %v", filepath.Base(path), parseErr)
			}
		})
	}
}
