package engine

import (
	"errors"
	"fmt"

	"github.com/tailflow/tailflow/internal/parser"
)

type DAGNode struct {
	Step     parser.Step
	Children []*DAGNode // nodes that depend on this one
	Parents  []*DAGNode // nodes this one depends on
}

type DAG struct {
	Nodes map[string]*DAGNode
	Roots []*DAGNode // nodes with no dependencies (entry points)
	Order []string   // topological order
}

func BuildDAG(steps []parser.Step) (*DAG, error) {
	dag := &DAG{
		Nodes: make(map[string]*DAGNode, len(steps)),
	}

	for _, s := range steps {
		dag.Nodes[s.ID] = &DAGNode{Step: s}
	}

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

	err = validateGotos(dag, steps)
	if err != nil {
		return nil, err
	}

	return dag, nil
}

func validateGotos(dag *DAG, steps []parser.Step) error {
	orderIndex := make(map[string]int, len(dag.Order))
	for i, id := range dag.Order {
		orderIndex[id] = i
	}

	for _, s := range steps {
		if s.Goto == nil {
			continue
		}

		_, ok := dag.Nodes[s.Goto.Target]
		if !ok {
			return fmt.Errorf("step %q goto references unknown step %q", s.ID, s.Goto.Target)
		}

		if orderIndex[s.Goto.Target] >= orderIndex[s.ID] {
			return fmt.Errorf("step %q goto target %q must be topologically before it", s.ID, s.Goto.Target)
		}

		if s.Goto.MaxIterations <= 0 {
			dag.Nodes[s.ID].Step.Goto.MaxIterations = 10
		}
	}

	return nil
}

func topoSort(dag *DAG) ([]string, error) {
	inDegree := make(map[string]int, len(dag.Nodes))
	for id, node := range dag.Nodes {
		inDegree[id] = len(node.Parents)
	}

	queue := make([]string, 0, len(dag.Roots))
	for _, root := range dag.Roots {
		queue = append(queue, root.Step.ID)
	}

	order := make([]string, 0, len(dag.Nodes))

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
