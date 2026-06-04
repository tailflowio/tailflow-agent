package server

import (
	"sort"
	"strings"

	"github.com/tailflow/tailflow/internal/engine"
	"github.com/tailflow/tailflow/internal/parser"
	"github.com/tailflow/tailflow/pkg/workflow"
)

func buildGraph(wf *parser.Workflow, dag *engine.DAG) workflow.Graph {
	graph := workflow.Graph{
		Nodes: []workflow.GraphNode{},
		Edges: []workflow.GraphEdge{},
	}

	stepIndex := make(map[string]parser.Step, len(wf.Steps))
	for _, s := range wf.Steps {
		stepIndex[s.ID] = s
	}

	loopTargets := make(map[string]bool)

	for _, s := range wf.Steps {
		if s.Goto != nil {
			loopTargets[s.Goto.Target] = true
		}
	}

	loopBodies := buildLoopBodies(wf, dag)

	visited := make(map[string]bool)
	var walk func(node *engine.DAGNode, depth int, parentID string, isLast bool)
	walk = func(node *engine.DAGNode, depth int, parentID string, isLast bool) {
		if visited[node.Step.ID] {
			return
		}

		visited[node.Step.ID] = true

		step := stepIndex[node.Step.ID]
		gn := buildGraphNode(step)
		gn.Depth = depth
		gn.ParentID = parentID
		gn.IsLast = isLast
		gn.InLoop = loopBodies[step.ID]
		gn.IsLoopStart = loopTargets[step.ID]

		if step.Goto != nil {
			gn.GotoTarget = step.Goto.Target
			gn.GotoMax = step.Goto.MaxIterations
		}

		graph.Nodes = append(graph.Nodes, gn)
		graph.Edges = append(graph.Edges, buildStepEdges(step)...)

		for i, child := range node.Children {
			last := i == len(node.Children)-1
			walk(child, depth+1, node.Step.ID, last)
		}
	}

	for i, root := range dag.Roots {
		last := i == len(dag.Roots)-1
		walk(root, 0, "", last)
	}

	// Sort nodes by YAML declaration order so the progress strip and any other
	// linear consumer see steps in the order the author wrote them. The DFS
	// above is needed to compute depth/parentID/IsLast for tree rendering, but
	// it leaves nodes in traversal order (e.g. a second root with shared
	// descendants ends up at the tail of the slice).
	yamlOrder := make(map[string]int, len(wf.Steps))
	for i, s := range wf.Steps {
		yamlOrder[s.ID] = i
	}

	sort.SliceStable(graph.Nodes, func(i, j int) bool {
		return yamlOrder[graph.Nodes[i].ID] < yamlOrder[graph.Nodes[j].ID]
	})

	graph.Tree = buildTreeLines(wf, dag)
	graph.Stages = buildStageInfos(wf)

	return graph
}

func buildStageInfos(wf *parser.Workflow) []workflow.StageInfo {
	stages := make([]workflow.StageInfo, 0, len(wf.Stages))

	for _, s := range wf.Stages {
		si := workflow.StageInfo{
			Name:        s.Name,
			Description: s.Description,
			Steps:       []string{},
		}

		for _, step := range wf.Steps {
			if step.Stage == s.Name {
				si.Steps = append(si.Steps, step.ID)
			}
		}

		stages = append(stages, si)
	}

	return stages
}

func buildLoopBodies(wf *parser.Workflow, dag *engine.DAG) map[string]bool {
	bodies := make(map[string]bool)

	for _, s := range wf.Steps {
		if s.Goto == nil {
			continue
		}

		current := s.Goto.Target
		visited := make(map[string]bool)

		for current != "" && !visited[current] {
			visited[current] = true
			bodies[current] = true

			if current == s.ID {
				break
			}

			node := dag.Nodes[current]
			if node != nil && len(node.Children) == 1 {
				current = node.Children[0].Step.ID
			} else {
				break
			}
		}
	}

	return bodies
}

func buildGraphNode(step parser.Step) workflow.GraphNode {
	label := step.Title
	if label == "" {
		label = step.ID
	}

	node := workflow.GraphNode{
		ID:         step.ID,
		Label:      label,
		Action:     step.Action,
		Type:       "step",
		When:       step.When,
		OnRecovery: step.OnRecovery,
	}

	if step.Action == "loop" {
		node.Pipeline = extractLoopPipeline(step.Config)
	}

	return node
}

func extractLoopPipeline(config map[string]any) []workflow.PipelineAction {
	rawActions, ok := config["actions"]
	if !ok {
		return nil
	}

	arr, ok := rawActions.([]any)
	if !ok {
		return nil
	}

	pipeline := make([]workflow.PipelineAction, 0, len(arr))

	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}

		actName, _ := m["action"].(string)
		actTitle, _ := m["title"].(string)

		if actName != "" {
			pipeline = append(pipeline, workflow.PipelineAction{
				Action: actName,
				Title:  actTitle,
			})
		}
	}

	if len(pipeline) == 0 {
		return nil
	}

	return pipeline
}

func buildStepEdges(step parser.Step) []workflow.GraphEdge {
	edges := make([]workflow.GraphEdge, 0, len(step.DependsOn)+1)

	for _, dep := range step.DependsOn {
		edge := workflow.GraphEdge{
			Source: dep,
			Target: step.ID,
		}

		if step.When != "" && strings.Contains(step.When, "steps."+dep+".") {
			edge.Type = "when"
			edge.Label = step.When
		}

		edges = append(edges, edge)
	}

	if step.Goto != nil {
		edges = append(edges, workflow.GraphEdge{
			Source: step.ID,
			Target: step.Goto.Target,
			Type:   "goto",
			Label:  step.Goto.When,
		})
	}

	return edges
}
