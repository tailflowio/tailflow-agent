package engine

import (
	"errors"
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
)

// DAGNode represents a node in the execution DAG.
type DAGNode struct {
	Step     parser.Step
	Children []*DAGNode // nodes that depend on this one
	Parents  []*DAGNode // nodes this one depends on
}

// DAG is a directed acyclic graph of workflow steps.
type DAG struct {
	Nodes map[string]*DAGNode
	Roots []*DAGNode // nodes with no dependencies (entry points)
	Order []string   // topological order
}

// BuildDAG constructs a DAG from workflow steps.
func BuildDAG(steps []parser.Step) (*DAG, error) {
	dag := &DAG{
		Nodes: make(map[string]*DAGNode, len(steps)),
	}

	// Create nodes
	for _, s := range steps {
		dag.Nodes[s.ID] = &DAGNode{Step: s}
	}

	// Build edges
	for _, s := range steps {
		node := dag.Nodes[s.ID]
		if len(s.DependsOn) == 0 {
			dag.Roots = append(dag.Roots, node)
			continue
		}

		for _, dep := range s.DependsOn {
			parent, ok := dag.Nodes[dep]
			if !ok {
				return nil, fmt.Errorf("step %q depends on unknown step %q", s.ID, dep)
			}

			parent.Children = append(parent.Children, node)
			node.Parents = append(node.Parents, parent)
		}
	}

	if len(dag.Roots) == 0 && len(steps) > 0 {
		return nil, errors.New("no root steps found (circular dependency?)")
	}

	// Topological sort (Kahn's algorithm)
	order, err := topoSort(dag)
	if err != nil {
		return nil, err
	}

	dag.Order = order

	// Validate goto references
	orderIndex := make(map[string]int, len(order))
	for i, id := range order {
		orderIndex[id] = i
	}

	for _, s := range steps {
		if s.Goto == nil {
			continue
		}

		if _, ok := dag.Nodes[s.Goto.Target]; !ok {
			return nil, fmt.Errorf("step %q goto references unknown step %q", s.ID, s.Goto.Target)
		}

		if orderIndex[s.Goto.Target] >= orderIndex[s.ID] {
			return nil, fmt.Errorf("step %q goto target %q must be topologically before it", s.ID, s.Goto.Target)
		}

		if s.Goto.MaxIterations <= 0 {
			dag.Nodes[s.ID].Step.Goto.MaxIterations = 10
		}
	}

	return dag, nil
}

// topoSort performs Kahn's algorithm for topological sorting.
func topoSort(dag *DAG) ([]string, error) {
	inDegree := make(map[string]int, len(dag.Nodes))
	for id, node := range dag.Nodes {
		inDegree[id] = len(node.Parents)
	}

	queue := make([]string, 0, len(dag.Roots))
	for _, root := range dag.Roots {
		queue = append(queue, root.Step.ID)
	}

	var order []string

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		order = append(order, id)

		node := dag.Nodes[id]
		for _, child := range node.Children {
			inDegree[child.Step.ID]--
			if inDegree[child.Step.ID] == 0 {
				queue = append(queue, child.Step.ID)
			}
		}
	}

	if len(order) != len(dag.Nodes) {
		return nil, errors.New("cycle detected in workflow DAG")
	}

	return order, nil
}

// FindLoopBody computes the set of nodes that form the loop body between
// targetID and gotoID. It is the intersection of forward-reachable nodes from
// target and backward-reachable nodes from gotoStep.
func FindLoopBody(dag *DAG, targetID, gotoID string) []string {
	// BFS forward from target → all descendants (+ itself)
	forward := map[string]bool{targetID: true}

	queue := []string{targetID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		for _, child := range dag.Nodes[id].Children {
			if !forward[child.Step.ID] {
				forward[child.Step.ID] = true

				queue = append(queue, child.Step.ID)
			}
		}
	}

	// BFS backward from gotoStep → all ancestors (+ itself)
	backward := map[string]bool{gotoID: true}

	queue = []string{gotoID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		for _, parent := range dag.Nodes[id].Parents {
			if !backward[parent.Step.ID] {
				backward[parent.Step.ID] = true

				queue = append(queue, parent.Step.ID)
			}
		}
	}

	// Intersection
	var body []string

	for id := range forward {
		if backward[id] {
			body = append(body, id)
		}
	}

	return body
}

// loopInDegree counts the number of parents of stepID that are within the bodySet.
func loopInDegree(dag *DAG, bodySet map[string]bool, stepID string) int {
	count := 0

	for _, parent := range dag.Nodes[stepID].Parents {
		if bodySet[parent.Step.ID] {
			count++
		}
	}

	return count
}
