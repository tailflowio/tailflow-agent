package server

import (
	"github.com/tailflow/tailflow/internal/event"
)

func (s *SSETestSuite) TestLoopTracker_TracksGotoEvents() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type:   event.StepGoto,
		StepID: "goto-step",
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step-a", "step-b"},
		},
	})

	s.Equal(2, lt.Iteration)
	s.True(lt.Body["step-a"])
	s.True(lt.Body["step-b"])
	s.False(lt.Body["step-c"])
}

func (s *SSETestSuite) TestLoopTracker_FiltersLoopBodyEvents() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type:   event.StepGoto,
		StepID: "goto-step",
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step-a", "step-b"},
		},
	})

	s.True(lt.InLoop(event.Event{
		Type:   event.StepStarted,
		StepID: "step-a",
	}), "step in loop body on iteration > 1 should be in-loop")

	s.True(lt.InLoop(event.Event{
		Type:   event.StepCompleted,
		StepID: "step-b",
	}), "step in loop body on iteration > 1 should be in-loop")

	s.False(lt.InLoop(event.Event{
		Type:   event.StepStarted,
		StepID: "step-c",
	}), "step NOT in loop body should not be in-loop")

	s.False(lt.InLoop(event.Event{
		Type:   event.StepGoto,
		StepID: "step-a",
	}), "step.goto events should never be considered in-loop")

	ltFirst := &loopTracker{}
	ltFirst.Track(event.Event{
		Type:   event.StepGoto,
		StepID: "goto-step",
		Data: map[string]any{
			"iteration": float64(1),
			"body":      []any{"step-a"},
		},
	})

	s.False(ltFirst.InLoop(event.Event{
		Type:   event.StepStarted,
		StepID: "step-a",
	}), "iteration 1 should NOT be filtered as in-loop")
}

func (s *SSETestSuite) TestLoopTracker_Track_GotoWithNilData() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: nil,
	})

	s.Equal(0, lt.Iteration)
	s.Nil(lt.Body)
}

func (s *SSETestSuite) TestLoopTracker_Track_GotoWithWrongTypes() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: map[string]any{
			"iteration": "not-a-number",
			"body":      "not-an-array",
		},
	})

	s.Equal(0, lt.Iteration)
	s.Nil(lt.Body)
}

func (s *SSETestSuite) TestLoopTracker_Track_GotoBodyWithNonStringElements() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{123, "step-a", nil},
		},
	})

	s.Equal(2, lt.Iteration)
	s.True(lt.Body["step-a"])
	s.Len(lt.Body, 1)
}

func (s *SSETestSuite) TestLoopTracker_Track_StepStartedResetsLoop() {
	lt := &loopTracker{}

	lt.Track(event.Event{
		Type: event.StepGoto,
		Data: map[string]any{
			"iteration": float64(2),
			"body":      []any{"step-a"},
		},
	})

	s.Equal(2, lt.Iteration)

	lt.Track(event.Event{
		Type:   event.StepStarted,
		StepID: "step-outside-loop",
	})

	s.Equal(0, lt.Iteration)
	s.Nil(lt.Body)
}
