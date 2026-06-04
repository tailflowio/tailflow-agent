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

func (s *ActionRegistryTestSuite) TestNewActionRegistry_DefaultAppliesSafeAllowlist() {
	out := NewActionRegistry(ActionRegistryIn{Config: Config{Unsafe: false}})
	s.NotNil(out.Registry)

	_, err := out.Registry.Create("exec")
	s.Error(err, "exec must be blocked under the default safe profile")
}

func (s *ActionRegistryTestSuite) TestNewActionRegistry_UnsafeKeepsEverything() {
	out := NewActionRegistry(ActionRegistryIn{Config: Config{Unsafe: true}})
	s.NotNil(out.Registry)

	_, err := out.Registry.Create("exec")
	s.NoError(err, "exec must be available under the unsafe profile")
}

func (s *ActionRegistryTestSuite) TestSafeAllowedActions_FiltersDangerousNames() {
	allowed := safeAllowedActions([]string{"http.get", "exec", "js", "file.read", "file.write", "wait.webhook"})
	s.NotContains(allowed, "exec")
	s.NotContains(allowed, "js")
	s.NotContains(allowed, "file.read")
	s.NotContains(allowed, "file.write")
	s.Contains(allowed, "http.get")
	s.Contains(allowed, "wait.webhook")
}
