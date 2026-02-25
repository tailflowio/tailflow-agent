package action

import (
	"errors"
	"fmt"
	"log/slog"
)

// LogAction logs a message at a specified level.
type LogAction struct{}

func NewLogAction() Action { return &LogAction{} }

func (a *LogAction) Validate(ctx *ActionContext) error {
	if _, ok := ctx.Config["message"]; !ok {
		return errors.New("log action requires 'message' in config")
	}

	return nil
}

func (a *LogAction) Execute(ctx *ActionContext) (any, error) {
	message := fmt.Sprintf("%v", ctx.Config["message"])

	level := "info"
	if l, ok := ctx.Config["level"]; ok {
		level = fmt.Sprintf("%v", l)
	}

	switch level {
	case "debug":
		ctx.Logger.Debug(message)
	case "info":
		ctx.Logger.Info(message)
	case "warn":
		ctx.Logger.Warn(message)
	case "error":
		ctx.Logger.Error(message)
	default:
		ctx.Logger.Log(ctx, slog.LevelInfo, message)
	}

	return map[string]any{"message": message, "level": level}, nil
}
