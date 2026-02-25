package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type MathActionTestSuite struct {
	suite.Suite
}

func TestMathAction(t *testing.T) {
	suite.Run(t, new(MathActionTestSuite))
}

func (s *MathActionTestSuite) SetupTest() {}

func (s *MathActionTestSuite) TestAllOps() {
	a := NewMathAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{10.0, 20.0, 30.0, 40.0, 50.0},
		"operations": []any{"sum", "min", "max", "avg", "count"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(150.0, m["sum"])
	s.Equal(10.0, m["min"])
	s.Equal(50.0, m["max"])
	s.Equal(30.0, m["avg"])
	s.Equal(5.0, m["count"])
}

func (s *MathActionTestSuite) TestWithField() {
	a := NewMathAction()
	ctx := newTestContext(map[string]any{
		"input": []any{
			map[string]any{"price": 10.0},
			map[string]any{"price": 20.0},
			map[string]any{"price": 30.0},
		},
		"operations": []any{"sum", "avg", "count"},
		"field":      "price",
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(60.0, m["sum"])
	s.Equal(20.0, m["avg"])
	s.Equal(3.0, m["count"])
}

func (s *MathActionTestSuite) TestEmptyArray() {
	a := NewMathAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{},
		"operations": []any{"sum", "min", "max", "avg", "count"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(0.0, m["sum"])
	s.Nil(m["min"])
	s.Nil(m["max"])
	s.Nil(m["avg"])
	s.Equal(0.0, m["count"])
}

func (s *MathActionTestSuite) TestStringNumbers() {
	a := NewMathAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{"10", "20.5", "30"},
		"operations": []any{"sum"},
	})

	out, err := a.Execute(ctx)
	s.Require().NoError(err)

	m := out.(map[string]any)
	s.Equal(60.5, m["sum"])
}

func (s *MathActionTestSuite) TestUnknownOp() {
	a := NewMathAction()
	ctx := newTestContext(map[string]any{
		"input":      []any{1.0},
		"operations": []any{"median"},
	})

	_, err := a.Execute(ctx)
	s.ErrorContains(err, "unknown operation")
}

func (s *MathActionTestSuite) TestValidateMissingInput() {
	a := NewMathAction()
	err := a.Validate(newTestContext(map[string]any{
		"operations": []any{"sum"},
	}))
	s.Error(err)
}

func (s *MathActionTestSuite) TestValidateMissingOps() {
	a := NewMathAction()
	err := a.Validate(newTestContext(map[string]any{
		"input": []any{1.0},
	}))
	s.Error(err)
}
