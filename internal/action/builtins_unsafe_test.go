package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BuiltinsUnsafeTestSuite struct {
	suite.Suite
}

func TestBuiltinsUnsafe(t *testing.T) {
	suite.Run(t, new(BuiltinsUnsafeTestSuite))
}

func (s *BuiltinsUnsafeTestSuite) SetupTest() {}

func (s *BuiltinsUnsafeTestSuite) TestRegisterBuiltinsIncludesUnsafe() {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	// In non-saas builds, unsafe actions should also be registered
	unsafeActions := []string{"exec", "js", "file.read", "file.write"}
	for _, name := range unsafeActions {
		s.True(reg.Has(name), "unsafe action %q should be registered in non-saas build", name)
	}
}

func (s *BuiltinsUnsafeTestSuite) TestNewScheduleAction() {
	a := NewScheduleAction()
	s.NotNil(a)
}
