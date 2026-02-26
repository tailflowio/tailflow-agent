package parser

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type SchemaTestSuite struct {
	suite.Suite
}

func TestSchema(t *testing.T) {
	suite.Run(t, new(SchemaTestSuite))
}

func (s *SchemaTestSuite) SetupTest() {}

func (s *SchemaTestSuite) TestRetryConfig_ParsedDelay() {
	r := &RetryConfig{MaxAttempts: 3, Delay: "5s"}
	d, err := r.ParsedDelay()
	s.Require().NoError(err)
	s.Equal(5_000_000_000, int(d))
}

func (s *SchemaTestSuite) TestRetryConfig_ParsedDelay_Default() {
	r := &RetryConfig{MaxAttempts: 3}
	d, err := r.ParsedDelay()
	s.Require().NoError(err)
	s.Equal(1_000_000_000, int(d))
}
