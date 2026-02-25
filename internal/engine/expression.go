package engine

// This file re-exports runtime types for backward compatibility.
// The actual implementation is in internal/runtime/expression.go

import "github.com/tailflow/tailflow/internal/runtime"

type ExprEvaluator = runtime.ExprEvaluator

var NewExprEvaluator = runtime.NewExprEvaluator
