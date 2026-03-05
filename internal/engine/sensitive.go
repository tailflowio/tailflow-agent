package engine

import "github.com/tailflow/tailflow/internal/event"

const RedactedValue = "[SENSITIVE]"

// SensitiveRegistry holds sensitive key names and masks them recursively.
type SensitiveRegistry struct {
	keys map[string]bool
}

func NewSensitiveRegistry(keys []string) *SensitiveRegistry {
	m := make(map[string]bool, len(keys))

	for _, k := range keys {
		m[k] = true
	}

	return &SensitiveRegistry{keys: m}
}

// MaskEvent returns a copy of the event with sensitive keys redacted in Data.
// If no sensitive keys are configured, returns the event as-is.
func (r *SensitiveRegistry) MaskEvent(ev event.Event) event.Event {
	if len(r.keys) == 0 || ev.Data == nil {
		return ev
	}

	masked := ev
	masked.Data = r.maskValue(ev.Data).(map[string]any)

	return masked
}

// MaskMap returns a copy of the map with sensitive keys redacted recursively.
// If no sensitive keys are configured, returns the original map.
func (r *SensitiveRegistry) MaskMap(m map[string]any) map[string]any {
	if len(r.keys) == 0 || m == nil {
		return m
	}

	return r.maskValue(m).(map[string]any)
}

func (r *SensitiveRegistry) maskValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))

		for k, v := range val {
			if r.keys[k] {
				out[k] = RedactedValue
			} else {
				out[k] = r.maskValue(v)
			}
		}

		return out
	case []any:
		out := make([]any, len(val))

		for i, item := range val {
			out[i] = r.maskValue(item)
		}

		return out
	default:
		return v
	}
}
