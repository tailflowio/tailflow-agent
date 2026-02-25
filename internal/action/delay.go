package action

import (
	"errors"
	"fmt"
	"time"
)

// DelayAction waits for a specified duration.
type DelayAction struct{}

func NewDelayAction() Action { return &DelayAction{} }

func (a *DelayAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["duration"]; !ok {
		return errors.New("delay action requires 'duration' in config")
	}

	return nil
}

func (a *DelayAction) Execute(ctx *ActionContext) (any, error) {
	durStr := fmt.Sprintf("%v", ctx.Config["duration"])

	dur, err := time.ParseDuration(durStr)
	if err != nil {
		return nil, fmt.Errorf("delay: invalid duration %q: %w", durStr, err)
	}

	select {
	case <-time.After(dur):
		return map[string]any{"waited": dur.String()}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
