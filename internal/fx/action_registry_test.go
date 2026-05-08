package fx

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ActionRegistryTestSuite struct {
	suite.Suite
}

func TestActionRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(ActionRegistryTestSuite))
}

func (s *ActionRegistryTestSuite) TestNewActionRegistry_DefaultAppliesSaaSAllowlist() {
	out := NewActionRegistry(ActionRegistryIn{Config: Config{SelfHosted: false}})
	s.NotNil(out.Registry)

	_, err := out.Registry.Create("exec")
	s.Error(err, "exec must be blocked under SaaS profile")
}

func (s *ActionRegistryTestSuite) TestNewActionRegistry_SelfHostedKeepsEverything() {
	out := NewActionRegistry(ActionRegistryIn{Config: Config{SelfHosted: true}})
	s.NotNil(out.Registry)

	_, err := out.Registry.Create("exec")
	s.NoError(err, "exec must be available in self-hosted profile")
}

func (s *ActionRegistryTestSuite) TestSaaSAllowedActions_FiltersDangerousNames() {
	allowed := saasAllowedActions([]string{"http.get", "exec", "js", "file.read", "file.write", "wait.webhook"})
	s.NotContains(allowed, "exec")
	s.NotContains(allowed, "js")
	s.NotContains(allowed, "file.read")
	s.NotContains(allowed, "file.write")
	s.Contains(allowed, "http.get")
	s.Contains(allowed, "wait.webhook")
}
