package action

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type HashActionTestSuite struct {
	suite.Suite
}

func TestHashAction(t *testing.T) {
	suite.Run(t, new(HashActionTestSuite))
}

func (s *HashActionTestSuite) SetupTest() {}

func (s *HashActionTestSuite) TestMissingInput() {
	a := NewHashAction()
	ctx := newTestContext(map[string]any{})
	err := a.Validate(ctx)
	s.Error(err)
	s.Contains(err.Error(), "input")
}

func (s *HashActionTestSuite) TestExecute_SHA256() {
	a := NewHashAction()
	ctx := newTestContext(map[string]any{"input": "hello world"})

	err := a.Validate(ctx)
	s.Require().NoError(err)

	out, err := a.Execute(ctx)
	s.Require().NoError(err)
	result := out.(map[string]any)

	// SHA-256 of "hello world"
	s.Equal("b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", result["hash"])
}

func (s *HashActionTestSuite) TestDeterministicOutput() {
	a := NewHashAction()
	ctx := newTestContext(map[string]any{"input": "même contenu RATP"})

	out1, err := a.Execute(ctx)
	s.Require().NoError(err)

	out2, err := a.Execute(ctx)
	s.Require().NoError(err)

	s.Equal(out1.(map[string]any)["hash"], out2.(map[string]any)["hash"])
}

func (s *HashActionTestSuite) TestDifferentInputDifferentHash() {
	a := NewHashAction()

	ctx1 := newTestContext(map[string]any{"input": "trafic normal"})
	out1, err := a.Execute(ctx1)
	s.Require().NoError(err)

	ctx2 := newTestContext(map[string]any{"input": "trafic perturbé"})
	out2, err := a.Execute(ctx2)
	s.Require().NoError(err)

	s.NotEqual(out1.(map[string]any)["hash"], out2.(map[string]any)["hash"])
}
