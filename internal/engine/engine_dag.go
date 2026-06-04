package engine

import (
	"context"
	"fmt"
	goruntime "runtime"
	"sync"

	"github.com/tailflow/tailflow/internal/event"
	"github.com/tailflow/tailflow/internal/runtime"
)

type loopInfo struct {
	body    []string
	bodySet map[string]bool
}

// dagState holds shared mutable state for a DAG execution.
type dagState struct {
	mu             sync.Mutex
	wg             sync.WaitGroup
	firstErr       error
	ready          chan *DAGNode
	done           chan struct{}
	completed      int
	total          int
	inDegree       map[string]int
	gotoLoops      map[string]loopInfo
	gotoIterations map[string]int
	sem            chan struct{}
}

func newDAGState(dag *DAG) *dagState {
	inDegree := make(map[string]int, len(dag.Nodes))
	for id, node := range dag.Nodes {
		inDegree[id] = len(node.Parents)
	}

	gotoLoops := make(map[string]loopInfo)

	for id, node := range dag.Nodes {
		if node.Step.Goto != nil {
			body := FindLoopBody(dag, node.Step.Goto.Target, id)
			bodySet := make(map[string]bool, len(body))

			for _, bid := range body {
				bodySet[bid] = true
			}

			gotoLoops[id] = loopInfo{body, bodySet}
		}
	}

	ready := make(chan *DAGNode, len(dag.Nodes))
	for _, root := range dag.Roots {
		ready <- root
	}

	return &dagState{
		ready:          ready,
		done:           make(chan struct{}),
		total:          len(dag.Nodes),
		inDegree:       inDegree,
		gotoLoops:      gotoLoops,
		gotoIterations: make(map[string]int),
		sem:            make(chan struct{}, goruntime.NumCPU()),
	}
}

func (e *Executor) executeDAG(ctx context.Context, dag *DAG, execCtx *runtime.ExecutionContext) error {
	ds := newDAGState(dag)

	go e.dagLoop(ctx, dag, execCtx, ds)

	<-ds.done
	ds.wg.Wait()

	return ds.firstErr
}

func (e *Executor) dagLoop(ctx context.Context, dag *DAG, execCtx *runtime.ExecutionContext, ds *dagState) {
	for node := range ds.ready {
		ds.mu.Lock()
		hasError := ds.firstErr != nil
		ds.mu.Unlock()

		if hasError {
			e.skipNode(node, execCtx, ds)

			continue
		}

		ds.wg.Add(1)

		ds.sem <- struct{}{}

		go e.runDAGNode(ctx, node, execCtx, dag, ds)
	}
}

func (e *Executor) skipNode(node *DAGNode, execCtx *runtime.ExecutionContext, ds *dagState) {
	execCtx.SetStepResult(node.Step.ID, &runtime.StepResult{Status: runtime.StatusSkipped})
	e.bus.Publish(event.NewEvent(event.StepSkipped, execCtx.ExecutionID, node.Step.ID, "skipped due to prior failure"))

	ds.mu.Lock()
	ds.completed++
	allDone := ds.completed >= ds.total
	ds.mu.Unlock()

	if allDone {
		close(ds.done)

		return
	}

	e.enqueueChildren(node.Children, ds)
}

func (e *Executor) runDAGNode(ctx context.Context, n *DAGNode, execCtx *runtime.ExecutionContext, dag *DAG, ds *dagState) {
	defer ds.wg.Done()
	defer func() { <-ds.sem }()

	err := e.executeNode(ctx, n, execCtx)

	ds.mu.Lock()
	if err != nil && ds.firstErr == nil {
		ds.firstErr = fmt.Errorf("step %q: %w", n.Step.ID, err)
	}

	ds.completed++

	if err == nil && n.Step.Goto != nil {
		readyNodes, triggered := e.handleGoto(n, execCtx, dag, ds.gotoLoops, ds.gotoIterations, ds.inDegree, &ds.completed)
		if triggered {
			ds.mu.Unlock()

			for _, rn := range readyNodes {
				ds.ready <- rn
			}

			return
		}
	}

	allDone := ds.completed >= ds.total
	ds.mu.Unlock()

	if allDone {
		close(ds.done)

		return
	}

	e.enqueueChildren(n.Children, ds)
}

func (e *Executor) enqueueChildren(children []*DAGNode, ds *dagState) {
	for _, child := range children {
		ds.mu.Lock()
		ds.inDegree[child.Step.ID]--
		shouldEnqueue := ds.inDegree[child.Step.ID] <= 0
		ds.mu.Unlock()

		if shouldEnqueue {
			ds.ready <- child
		}
	}
}
