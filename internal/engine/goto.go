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

// handleGoto evaluates the goto condition for a node that just completed successfully,
// and if triggered, resets the loop body and enqueues the ready nodes.
// Returns true if a goto was triggered (caller should NOT propagate to children).
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

	// Reset loop body: clear results and recompute in-degrees
	for _, bid := range li.body {
		inDegree[bid] = loopInDegree(dag, li.bodySet, bid)
		*completed--

		execCtx.ClearStepResult(bid)
	}

	// Collect nodes in the body with inDegree == 0
	for _, bid := range li.body {
		if inDegree[bid] == 0 {
			readyNodes = append(readyNodes, dag.Nodes[bid])
		}
	}

	// Emit step.goto event
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
			"body":           li.body,
		},
	})

	return readyNodes, true
}
