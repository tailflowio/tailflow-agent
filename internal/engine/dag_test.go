package engine

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

type DAGTestSuite struct {
	suite.Suite
}

func TestDAG(t *testing.T) {
	suite.Run(t, new(DAGTestSuite))
}

func (s *DAGTestSuite) SetupTest() {}

func (s *DAGTestSuite) TestBuildDAG_Linear() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"b"}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	s.Len(dag.Roots, 1)
	s.Equal("a", dag.Roots[0].Step.ID)
	s.Equal([]string{"a", "b", "c"}, dag.Order)
}

func (s *DAGTestSuite) TestBuildDAG_Parallel() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log"},
		{ID: "c", Action: "log", DependsOn: []string{"a", "b"}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	s.Len(dag.Roots, 2)
	s.Len(dag.Order, 3)
	s.Equal("c", dag.Order[2]) // c must come last
}

func (s *DAGTestSuite) TestBuildDAG_Diamond() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"a"}},
		{ID: "d", Action: "log", DependsOn: []string{"b", "c"}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	s.Len(dag.Roots, 1)
	s.Equal("a", dag.Order[0])
	s.Equal("d", dag.Order[3])
}

func (s *DAGTestSuite) TestBuildDAG_CycleDetection() {
	steps := []parser.Step{
		{ID: "a", Action: "log", DependsOn: []string{"c"}},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"b"}},
	}
	_, err := BuildDAG(steps)
	s.Require().Error(err)
	s.Contains(err.Error(), "no root steps found")
}

func (s *DAGTestSuite) TestBuildDAG_UnknownDependency() {
	steps := []parser.Step{
		{ID: "a", Action: "log", DependsOn: []string{"nonexistent"}},
	}
	_, err := BuildDAG(steps)
	s.Require().Error(err)
	s.Contains(err.Error(), "unknown step")
}

func (s *DAGTestSuite) TestBuildDAG_SingleNode() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	s.Len(dag.Roots, 1)
	s.Equal([]string{"a"}, dag.Order)
}

func (s *DAGTestSuite) TestBuildDAG_GotoValid() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"b"}},
		{ID: "d", Action: "log", DependsOn: []string{"c"}, Goto: &parser.GotoConfig{Target: "b", When: "true", MaxIterations: 3}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)
	s.Equal(3, dag.Nodes["d"].Step.Goto.MaxIterations)
}

func (s *DAGTestSuite) TestBuildDAG_GotoUnknownTarget() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}, Goto: &parser.GotoConfig{Target: "nonexistent", When: "true"}},
	}
	_, err := BuildDAG(steps)
	s.Require().Error(err)
	s.Contains(err.Error(), "unknown step")
}

func (s *DAGTestSuite) TestBuildDAG_GotoForwardTarget() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}, Goto: &parser.GotoConfig{Target: "c", When: "true"}},
		{ID: "c", Action: "log", DependsOn: []string{"b"}},
	}
	_, err := BuildDAG(steps)
	s.Require().Error(err)
	s.Contains(err.Error(), "must be topologically before")
}

func (s *DAGTestSuite) TestBuildDAG_GotoDefaultMaxIterations() {
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}, Goto: &parser.GotoConfig{Target: "a", When: "true"}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)
	s.Equal(10, dag.Nodes["b"].Step.Goto.MaxIterations)
}

func (s *DAGTestSuite) TestFindLoopBody() {
	// A -> B -> C -> D, goto(D->B) = {B, C, D}
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"b"}},
		{ID: "d", Action: "log", DependsOn: []string{"c"}, Goto: &parser.GotoConfig{Target: "b", When: "true", MaxIterations: 3}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	body := FindLoopBody(dag, "b", "d")
	bodySet := make(map[string]bool)
	for _, id := range body {
		bodySet[id] = true
	}
	s.True(bodySet["b"])
	s.True(bodySet["c"])
	s.True(bodySet["d"])
	s.False(bodySet["a"])
	s.Len(body, 3)
}

func (s *DAGTestSuite) TestFindLoopBody_Diamond() {
	// A -> (B, C) -> D, goto(D->B) = {B, D} (C is outside the body)
	steps := []parser.Step{
		{ID: "a", Action: "log"},
		{ID: "b", Action: "log", DependsOn: []string{"a"}},
		{ID: "c", Action: "log", DependsOn: []string{"a"}},
		{ID: "d", Action: "log", DependsOn: []string{"b", "c"}, Goto: &parser.GotoConfig{Target: "b", When: "true", MaxIterations: 3}},
	}
	dag, err := BuildDAG(steps)
	s.Require().NoError(err)

	body := FindLoopBody(dag, "b", "d")
	bodySet := make(map[string]bool)
	for _, id := range body {
		bodySet[id] = true
	}
	s.True(bodySet["b"])
	s.True(bodySet["d"])
	s.False(bodySet["a"])
	s.False(bodySet["c"])
	s.Len(body, 2)
}
