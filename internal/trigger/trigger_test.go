package trigger

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/parser"
)

type TriggerTestSuite struct {
	suite.Suite
}

func TestTrigger(t *testing.T) {
	suite.Run(t, new(TriggerTestSuite))
}

func (s *TriggerTestSuite) SetupTest() {
	// required by convention
}

func (s *TriggerTestSuite) TestResolveRoutes() {
	workflows := []*parser.Workflow{
		{
			Name: "api",
			Trigger: &parser.Trigger{
				HTTP: &parser.HTTPTrigger{Method: "POST", Path: "/users"},
			},
		},
		{
			Name: "webhook",
			Trigger: &parser.Trigger{
				Webhook: &parser.WebhookTrigger{Path: "/github", Secret: "secret123"},
			},
		},
		{
			Name: "manual",
		},
	}

	routes := ResolveRoutes(workflows)
	s.Len(routes, 2)
	s.Equal("api", routes[0].WorkflowName)
	s.Equal("http", routes[0].TriggerType)
	s.Equal("/users", routes[0].Path)
	s.Equal("webhook", routes[1].WorkflowName)
	s.Equal("webhook", routes[1].TriggerType)
	s.Equal("secret123", routes[1].Secret)
}

func (s *TriggerTestSuite) TestMatchRoute() {
	routes := []Route{
		{WorkflowName: "api", Method: "POST", Path: "/users", TriggerType: "http"},
		{WorkflowName: "hook", Method: "POST", Path: "/github", TriggerType: "webhook"},
	}

	r, err := MatchRoute(routes, "POST", "/users")
	s.Require().NoError(err)
	s.Equal("api", r.WorkflowName)

	r, err = MatchRoute(routes, "POST", "/github")
	s.Require().NoError(err)
	s.Equal("hook", r.WorkflowName)

	_, err = MatchRoute(routes, "GET", "/users")
	s.Error(err)

	_, err = MatchRoute(routes, "POST", "/nonexistent")
	s.Error(err)
}
