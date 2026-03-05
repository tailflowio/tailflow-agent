package action

import (
	"errors"
	"fmt"
	"time"
)

// ScheduleAction schedules a future workflow execution.
type ScheduleAction struct{}

func NewScheduleAction() Action { return &ScheduleAction{} }

func (a *ScheduleAction) Validate(ctx *ActionContext) error {
	delay, hasDelay := ctx.Config["delay"]
	at, hasAt := ctx.Config["at"]

	if hasDelay && hasAt {
		return errors.New("schedule: cannot specify both 'delay' and 'at'")
	}

	if !hasDelay && !hasAt {
		return errors.New("schedule: must specify 'delay' or 'at'")
	}

	if hasDelay {
		s, ok := delay.(string)
		if !ok {
			return errors.New("schedule: 'delay' must be a string duration")
		}

		_, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("schedule: invalid delay %q: %w", s, err)
		}
	}

	if hasAt {
		s, ok := at.(string)
		if !ok {
			return errors.New("schedule: 'at' must be an RFC3339 string")
		}

		_, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("schedule: invalid 'at' %q: %w", s, err)
		}
	}

	return nil
}

func (a *ScheduleAction) Execute(ctx *ActionContext) (any, error) {
	if ctx.Services == nil || ctx.Services.ScheduleExecution == nil {
		return nil, errors.New("schedule: requires server mode (use 'tailflow serve')")
	}

	d := computeDelay(ctx.Config)

	// Collect params from config
	params := extractParams(ctx.Config)

	executionID, err := ctx.Services.ScheduleExecution(d, params)
	if err != nil {
		return nil, fmt.Errorf("schedule: %w", err)
	}

	scheduledAt := time.Now().Add(d)

	return map[string]any{
		"scheduled_at": scheduledAt.Format(time.RFC3339),
		"execution_id": executionID,
	}, nil
}

func computeDelay(config map[string]any) time.Duration {
	delayStr, ok := config["delay"].(string)
	if ok {
		d, _ := time.ParseDuration(delayStr)
		return d
	}

	atStr, ok := config["at"].(string)
	if ok {
		t, _ := time.Parse(time.RFC3339, atStr)

		d := time.Until(t)
		if d < 0 {
			return 0
		}

		return d
	}

	return 0
}

func extractParams(config map[string]any) map[string]any {
	p, ok := config["params"]
	if !ok {
		return nil
	}

	m, ok := p.(map[string]any)
	if !ok {
		return nil
	}

	return m
}
