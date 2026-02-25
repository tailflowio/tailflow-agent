package server

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type RabbitMQWaitTestSuite struct {
	suite.Suite
}

func TestRabbitMQWait(t *testing.T) {
	suite.Run(t, new(RabbitMQWaitTestSuite))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_TopLevel() {
	m := map[string]any{"order_id": "abc-123"}
	s.Equal("abc-123", extractDotField(m, "order_id"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_Nested() {
	m := map[string]any{
		"data": map[string]any{
			"order": map[string]any{
				"id": 42,
			},
		},
	}
	s.Equal("42", extractDotField(m, "data.order.id"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_Missing() {
	m := map[string]any{"foo": "bar"}
	s.Equal("", extractDotField(m, "missing"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_MissingNested() {
	m := map[string]any{"foo": "bar"}
	s.Equal("", extractDotField(m, "foo.bar.baz"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_EmptyMap() {
	m := map[string]any{}
	s.Equal("", extractDotField(m, "anything"))
}

func (s *RabbitMQWaitTestSuite) TestExtractDotField_StringValue() {
	m := map[string]any{"status": "confirmed"}
	s.Equal("confirmed", extractDotField(m, "status"))
}
