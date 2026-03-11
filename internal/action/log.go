package action

import (
	"errors"
	"fmt"
	"log/slog"
)

type LogAction struct{}

func NewLogAction() Action { return &LogAction{} }

func (a *LogAction) Validate(ctx *ActionContext) error {
	_, ok := ctx.Config["message"]
	if !ok {
		return errors.New("log action requires 'message' in config")
	}

	return nil
}

func (a *LogAction) Execute(ctx *ActionContext) (any, error) {
	message := fmt.Sprintf("%v", ctx.Config["message"])

	level := "info"

	l, ok := ctx.Config["level"]
	if ok {
		level = fmt.Sprintf("%v", l)
	}

	if isStream(ctx) {
		ctx.EmitLog(message)

		return map[string]any{"message": message, "level": level}, nil
	}

	logWithLevel(ctx, level, message)

	return map[string]any{"message": message, "level": level}, nil
}

func isStream(ctx *ActionContext) bool {
	s, ok := ctx.Config["stream"]
	if !ok {
		return false
	}

	b, ok := s.(bool)

	return ok && b
}

func logWithLevel(ctx *ActionContext, level, message string) {
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
}
