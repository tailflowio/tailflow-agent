package engine

// This file re-exports runtime types for backward compatibility.
// The actual implementation is in internal/runtime/context.go

import "github.com/tailflow/tailflow/internal/runtime"

// Type aliases to avoid breaking changes.
type (
	ExecutionContext = runtime.ExecutionContext
	StepResult       = runtime.StepResult
)

var NewExecutionContext = runtime.NewExecutionContext
