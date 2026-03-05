package engine

import (
	"fmt"
	"time"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

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
	body := make([]string, 0, len(forward))

	for id := range forward {
		if backward[id] {
			body = append(body, id)
		}
	}

	return body
}

func loopInDegree(dag *DAG, bodySet map[string]bool, stepID string) int {
	count := 0

	for _, parent := range dag.Nodes[stepID].Parents {
		if bodySet[parent.Step.ID] {
			count++
		}
	}

	return count
}

func (e *Executor) handleGoto(
	n *DAGNode,
	execCtx *runtime.ExecutionContext,
	dag *DAG,
	gotoLoops map[string]loopInfo,
	gotoIterations map[string]int,
	inDegree map[string]int,
	completed *int,
) (readyNodes []*DAGNode, triggered bool) {
	li, hasLoop := gotoLoops[n.Step.ID]
	if !hasLoop {
		return nil, false
	}

	maxIter := n.Step.Goto.MaxIterations
	gotoResult, evalErr := e.eval.EvalBool(n.Step.Goto.When, execCtx.ToMap())

	if evalErr != nil || !gotoResult || gotoIterations[n.Step.ID] >= maxIter {
		return nil, false
	}

	gotoIterations[n.Step.ID]++
	iter := gotoIterations[n.Step.ID]

	resetLoopBody(dag, li, inDegree, completed, execCtx)
	readyNodes = collectReadyLoopNodes(dag, li, inDegree)

	e.publishGotoEvent(execCtx, n, iter, maxIter, li.body)

	return readyNodes, true
}

func resetLoopBody(
	dag *DAG, li loopInfo, inDegree map[string]int,
	completed *int, execCtx *runtime.ExecutionContext,
) {
	for _, bid := range li.body {
		inDegree[bid] = loopInDegree(dag, li.bodySet, bid)
		*completed--

		execCtx.ClearStepResult(bid)
	}
}

func collectReadyLoopNodes(dag *DAG, li loopInfo, inDegree map[string]int) []*DAGNode {
	var ready []*DAGNode

	for _, bid := range li.body {
		if inDegree[bid] == 0 {
			ready = append(ready, dag.Nodes[bid])
		}
	}

	return ready
}

func (e *Executor) publishGotoEvent(
	execCtx *runtime.ExecutionContext, n *DAGNode, iter, maxIter int, body []string,
) {
	e.bus.Publish(event.Event{
		Type:        event.StepGoto,
		Timestamp:   time.Now(),
		ExecutionID: execCtx.ExecutionID,
		StepID:      n.Step.ID,
		Message:     fmt.Sprintf("goto %s (iteration %d/%d)", n.Step.Goto.Target, iter+1, maxIter),
		Data: map[string]any{
			"target":         n.Step.Goto.Target,
			"iteration":      iter + 1,
			"max_iterations": maxIter,
			"body":           body,
		},
	})
}
