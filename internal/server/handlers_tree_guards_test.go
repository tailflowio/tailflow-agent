package server

import (
	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
)

// TestResolveConvergent_NilStep covers the guard at line 118: st == nil.
// This fires when a parent node's step ID is present in dag.Nodes but
// absent from r.steps — the builder treats it as a non-sibling and skips.
func (s *HandlersTestSuite) TestResolveConvergent_NilStep() {
	parentA := &engine.DAGNode{Step: parser.Step{ID: "a"}}
	parentB := &engine.DAGNode{Step: parser.Step{ID: "b"}}
	child := &engine.DAGNode{
		Step:    parser.Step{ID: "c"},
		Parents: []*engine.DAGNode{parentA, parentB},
	}

	dag := &engine.DAG{
		Nodes: map[string]*engine.DAGNode{
			"a": parentA,
			"b": parentB,
			"c": child,
		},
	}

	// "a" is in r.steps but "b" is intentionally absent — triggers st == nil guard.
	r := &treeBuilder{
		order: []string{"a", "c"},
		steps: map[string]*treeStep{
			"a": {id: "a", parentID: ""},
			"c": {id: "c", parentID: "a"},
		},
	}

	r.resolveConvergent(dag)

	s.NotNil(r.steps["c"])
}

// TestResolveConvergent_NilDAGNode covers the guard at line 151: dn == nil.
// This fires inside the collect closure when a child's step ID is referenced
// by dag.Nodes but the node itself is absent — the closure returns early.
func (s *HandlersTestSuite) TestResolveConvergent_NilDAGNode() {
	parentA := &engine.DAGNode{Step: parser.Step{ID: "a"}}
	parentB := &engine.DAGNode{Step: parser.Step{ID: "b"}}

	ghost := &engine.DAGNode{Step: parser.Step{ID: "ghost"}}

	child := &engine.DAGNode{
		Step:    parser.Step{ID: "c"},
		Parents: []*engine.DAGNode{parentA, parentB},
		// ghost is listed as a child but will NOT be present in dag.Nodes.
		Children: []*engine.DAGNode{ghost},
	}

	parentA.Children = []*engine.DAGNode{child}
	parentB.Children = []*engine.DAGNode{child}

	dag := &engine.DAG{
		Nodes: map[string]*engine.DAGNode{
			"a": parentA,
			"b": parentB,
			"c": child,
			// "ghost" deliberately absent — triggers dn == nil guard.
		},
	}

	r := &treeBuilder{
		order: []string{"a", "b", "c"},
		steps: map[string]*treeStep{
			"a": {id: "a", depth: 0, parentID: ""},
			"b": {id: "b", depth: 0, parentID: ""},
			"c": {id: "c", depth: 1, parentID: "a"},
		},
	}

	r.resolveConvergent(dag)

	s.NotNil(r.steps["c"])
}

// TestResolveConvergent_LastParentInDescSet covers the guard at line 184: lastParentIdx == -1.
// This fires when the last convergent parent (in topological order) is itself
// recorded as a descendant of the convergent node — an incoherent but
// defensively guarded input.  "b" is a parent of "c" AND a child of "c"
// in the DAG Children list, causing collect() to put "b" into descSet,
// so "b" never appears in remaining, leaving lastParentIdx == -1.
func (s *HandlersTestSuite) TestResolveConvergent_LastParentInDescSet() {
	root := &engine.DAGNode{Step: parser.Step{ID: "root"}}
	parentA := &engine.DAGNode{Step: parser.Step{ID: "a"}, Parents: []*engine.DAGNode{root}}
	// "b" is deliberately BOTH a parent and a child of "c" (incoherent).
	parentB := &engine.DAGNode{Step: parser.Step{ID: "b"}, Parents: []*engine.DAGNode{root}}

	child := &engine.DAGNode{
		Step:    parser.Step{ID: "c"},
		Parents: []*engine.DAGNode{parentA, parentB},
		// Listing "b" as a child causes collect("c") to add "b" to descSet.
		Children: []*engine.DAGNode{parentB},
	}

	root.Children = []*engine.DAGNode{parentA, parentB}
	parentA.Children = []*engine.DAGNode{child}

	dag := &engine.DAG{
		Nodes: map[string]*engine.DAGNode{
			"root": root,
			"a":    parentA,
			"b":    parentB,
			"c":    child,
		},
	}

	// Both "a" and "b" share the same parentID ("root") so allSiblings remains true.
	// "b" appears after "c" in r.order so it is sorted as the last parent;
	// because "b" is in descSet it will not appear in remaining → lastParentIdx == -1.
	r := &treeBuilder{
		order: []string{"root", "a", "c", "b"},
		steps: map[string]*treeStep{
			"root": {id: "root", depth: 0, parentID: ""},
			"a":    {id: "a", depth: 1, parentID: "root"},
			"b":    {id: "b", depth: 1, parentID: "root"},
			"c":    {id: "c", depth: 2, parentID: "a"},
		},
	}

	r.resolveConvergent(dag)

	s.NotNil(r.steps["c"])
}
