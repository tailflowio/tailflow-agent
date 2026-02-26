package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BuiltinsTestSuite struct {
	suite.Suite
}

func TestBuiltins(t *testing.T) {
	suite.Run(t, new(BuiltinsTestSuite))
}

func (s *BuiltinsTestSuite) SetupTest() {}

func (s *BuiltinsTestSuite) TestRegisterBuiltins() {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	// Verify all expected safe actions are registered
	expectedActions := []string{
		"set", "log", "http", "condition", "loop",
		"json.decode", "json.encode", "template", "delay",
		"response", "wait.webhook", "wait.rabbitmq",
		"rabbitmq.shovel", "validate", "lock", "unlock",
		"sql.query", "sql.exec", "sql.begin", "sql.commit", "sql.rollback",
		"schedule", "array.sort", "array.filter", "array.map",
		"array.uniq", "array.pick", "array.concat",
		"object", "math", "hash", "string.replace", "string.match_all",
		"kv.get", "kv.set", "kv.delete",
	}

	for _, name := range expectedActions {
		s.True(reg.Has(name), "action %q should be registered", name)
		a, err := reg.Create(name)
		s.NoError(err, "action %q should be createable", name)
		s.NotNil(a, "action %q should not be nil", name)
	}
}
