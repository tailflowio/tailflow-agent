package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ObjectActionTestSuite struct {
	suite.Suite
}

func TestObjectAction(t *testing.T) {
	suite.Run(t, new(ObjectActionTestSuite))
}

func (s *ObjectActionTestSuite) SetupTest() {}

func (s *ObjectActionTestSuite) TestKeys() {
	a := NewObjectAction()
	ctx := newTestContext(map[string]any{
		"name":   "Alice",
		"email":  "alice@example.com",
		"active": true,
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("Alice", m["name"])
	s.Equal("alice@example.com", m["email"])
	s.Equal(true, m["active"])
}

func (s *ObjectActionTestSuite) TestKeysIgnoresMode() {
	a := NewObjectAction()
	ctx := newTestContext(map[string]any{
		"name": "Bob",
		"mode": "keys",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("Bob", m["name"])
	_, hasMode := m["mode"]
	s.False(hasMode)
}

func (s *ObjectActionTestSuite) TestMerge() {
	a := NewObjectAction()
	ctx := newTestContext(map[string]any{
		"mode": "merge",
		"objects": []any{
			map[string]any{"name": "Alice", "age": 30},
			map[string]any{"email": "alice@example.com", "age": 31},
		},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal("Alice", m["name"])
	s.Equal("alice@example.com", m["email"])
	s.Equal(31, m["age"]) // last wins
}

func (s *ObjectActionTestSuite) TestMergeEmpty() {
	a := NewObjectAction()
	ctx := newTestContext(map[string]any{
		"mode":    "merge",
		"objects": []any{},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Len(m, 0)
}

func (s *ObjectActionTestSuite) TestMergeInvalidItem() {
	a := NewObjectAction()
	ctx := newTestContext(map[string]any{
		"mode":    "merge",
		"objects": []any{"not an object"},
	})

	_, err := a.Execute(ctx)
	s.ErrorContains(err, "not an object")
}

func (s *ObjectActionTestSuite) TestValidateMergeMissingObjects() {
	a := NewObjectAction()
	err := a.Validate(newTestContext(map[string]any{"mode": "merge"}))
	s.Error(err)
}

func (s *ObjectActionTestSuite) TestValidateKeysModeOK() {
	a := NewObjectAction()
	err := a.Validate(newTestContext(map[string]any{"name": "test"}))
	s.NoError(err)
}
