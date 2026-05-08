package fx

import (
	"context"

	"github.com/tailflow/tailflow/internal/event"
	uberfx "go.uber.org/fx"
)

type EventBusIn struct {
	uberfx.In

	Lifecycle uberfx.Lifecycle
}

type EventBusOut struct {
	uberfx.Out

	EventBus *event.Bus
}

func NewEventBus(in EventBusIn) EventBusOut {
	bus := event.NewBus()

	in.Lifecycle.Append(uberfx.Hook{
		OnStop: func(_ context.Context) error {
			bus.Close()
			return nil
		},
	})

	return EventBusOut{EventBus: bus}
}
