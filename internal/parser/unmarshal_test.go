package parser

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"
)

type UnmarshalTestSuite struct {
	suite.Suite
}

func TestUnmarshal(t *testing.T) {
	suite.Run(t, new(UnmarshalTestSuite))
}

func (s *UnmarshalTestSuite) SetupTest() {}

func (s *UnmarshalTestSuite) TestStepUnmarshalYAML_CapturesLine() {
	data := `
version: "2.0"
name: line-test
stages:
  - name: default
steps:
  - id: a
    stage: default
    action: log
`
	w, err := ParseBytes([]byte(data))
	s.Require().NoError(err)
	s.Equal(7, w.Steps[0].Line)
}

func (s *UnmarshalTestSuite) TestStepUnmarshalYAML_DecodeError() {
	var step Step

	err := yaml.Unmarshal([]byte("just-a-scalar"), &step)
	s.Error(err)
}

func (s *UnmarshalTestSuite) TestStageUnmarshalYAML_CapturesLine() {
	data := `
version: "2.0"
name: line-test
stages:
  - name: default
steps:
  - id: a
    stage: default
    action: log
`
	w, err := ParseBytes([]byte(data))
	s.Require().NoError(err)
	s.Equal(5, w.Stages[0].Line)
}

func (s *UnmarshalTestSuite) TestStageUnmarshalYAML_DecodeError() {
	var stage Stage

	err := yaml.Unmarshal([]byte("just-a-scalar"), &stage)
	s.Error(err)
}

func (s *UnmarshalTestSuite) TestParamUnmarshalYAML_CapturesLine() {
	data := `
version: "2.0"
name: line-test
params:
  - name: host
    type: string
stages:
  - name: default
steps:
  - id: a
    stage: default
    action: log
`
	w, err := ParseBytes([]byte(data))
	s.Require().NoError(err)
	s.Equal(5, w.Params[0].Line)
}

func (s *UnmarshalTestSuite) TestParamUnmarshalYAML_DecodeError() {
	var param Param

	err := yaml.Unmarshal([]byte("just-a-scalar"), &param)
	s.Error(err)
}
