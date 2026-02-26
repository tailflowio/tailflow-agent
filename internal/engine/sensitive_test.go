package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/tailflow/tailflow/internal/event"
)

type SensitiveTestSuite struct {
	suite.Suite
}

func TestSensitive(t *testing.T) {
	suite.Run(t, new(SensitiveTestSuite))
}

func (s *SensitiveTestSuite) SetupTest() {
	// required by convention
}

func (s *SensitiveTestSuite) TestNewSensitiveRegistry() {
	r := NewSensitiveRegistry([]string{"token", "password"})

	s.True(r.keys["token"])
	s.True(r.keys["password"])
	s.False(r.keys["username"])
}

func (s *SensitiveTestSuite) TestNewSensitiveRegistry_Empty() {
	r := NewSensitiveRegistry(nil)

	s.Empty(r.keys)
}

func (s *SensitiveTestSuite) TestMaskEvent_StepOutput() {
	r := NewSensitiveRegistry([]string{"token", "api_key"})
	ev := event.Event{
		Type:        event.StepOutput,
		Timestamp:   time.Now(),
		ExecutionID: "exec-1",
		StepID:      "step-1",
		Data: map[string]any{
			"output": map[string]any{
				"token":   "secret-123",
				"api_key": "key-456",
				"status":  "ok",
			},
		},
	}

	masked := r.MaskEvent(ev)

	output := masked.Data["output"].(map[string]any)
	s.Equal(RedactedValue, output["token"])
	s.Equal(RedactedValue, output["api_key"])
	s.Equal("ok", output["status"])

	// Original event must be untouched
	origOutput := ev.Data["output"].(map[string]any)
	s.Equal("secret-123", origOutput["token"])
}

func (s *SensitiveTestSuite) TestMaskEvent_NestedKeys() {
	r := NewSensitiveRegistry([]string{"password"})
	ev := event.Event{
		Type: event.StepOutput,
		Data: map[string]any{
			"output": map[string]any{
				"user": map[string]any{
					"name": "alice",
					"auth": map[string]any{
						"password": "s3cret",
						"method":   "basic",
					},
				},
			},
		},
	}

	masked := r.MaskEvent(ev)

	auth := masked.Data["output"].(map[string]any)["user"].(map[string]any)["auth"].(map[string]any)
	s.Equal(RedactedValue, auth["password"])
	s.Equal("basic", auth["method"])
}

func (s *SensitiveTestSuite) TestMaskEvent_ArrayOfObjects() {
	r := NewSensitiveRegistry([]string{"card_number"})
	ev := event.Event{
		Type: event.StepOutput,
		Data: map[string]any{
			"output": []any{
				map[string]any{"card_number": "4111-1111-1111-1111", "type": "visa"},
				map[string]any{"card_number": "5500-0000-0000-0004", "type": "mastercard"},
			},
		},
	}

	masked := r.MaskEvent(ev)

	items := masked.Data["output"].([]any)
	for _, item := range items {
		m := item.(map[string]any)
		s.Equal(RedactedValue, m["card_number"])
	}
}

func (s *SensitiveTestSuite) TestMaskEvent_NoSensitiveKeys() {
	r := NewSensitiveRegistry(nil)
	ev := event.Event{
		Type: event.StepOutput,
		Data: map[string]any{"token": "secret"},
	}

	masked := r.MaskEvent(ev)

	s.Equal("secret", masked.Data["token"])
}

func (s *SensitiveTestSuite) TestMaskEvent_NoMatchingKeys() {
	r := NewSensitiveRegistry([]string{"password"})
	ev := event.Event{
		Type: event.StepOutput,
		Data: map[string]any{
			"username": "alice",
			"role":     "admin",
		},
	}

	masked := r.MaskEvent(ev)

	s.Equal("alice", masked.Data["username"])
	s.Equal("admin", masked.Data["role"])
}

func (s *SensitiveTestSuite) TestMaskEvent_NilData() {
	r := NewSensitiveRegistry([]string{"token"})
	ev := event.Event{
		Type: event.StepOutput,
		Data: nil,
	}

	masked := r.MaskEvent(ev)

	s.Nil(masked.Data)
}

func (s *SensitiveTestSuite) TestMaskMap_Masks() {
	r := NewSensitiveRegistry([]string{"token", "api_key"})
	m := map[string]any{
		"token":  "secret",
		"status": "ok",
		"nested": map[string]any{"api_key": "key-456"},
	}

	masked := r.MaskMap(m)

	s.Equal(RedactedValue, masked["token"])
	s.Equal("ok", masked["status"])
	s.Equal(RedactedValue, masked["nested"].(map[string]any)["api_key"])

	// Original untouched
	s.Equal("secret", m["token"])
}

func (s *SensitiveTestSuite) TestMaskMap_NoKeys() {
	r := NewSensitiveRegistry(nil)
	m := map[string]any{"token": "secret"}

	s.Equal(m, r.MaskMap(m))
}

func (s *SensitiveTestSuite) TestMaskMap_NilMap() {
	r := NewSensitiveRegistry([]string{"token"})

	s.Nil(r.MaskMap(nil))
}

func (s *SensitiveTestSuite) TestMaskValue_ScalarUntouched() {
	r := NewSensitiveRegistry([]string{"token"})

	s.Equal("hello", r.maskValue("hello"))
	s.Equal(42, r.maskValue(42))
	s.Equal(true, r.maskValue(true))
	s.Nil(r.maskValue(nil))
}
