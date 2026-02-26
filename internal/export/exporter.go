package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tailflow/tailflow/internal/event"
)

type Config struct {
	ExportURL           string
	APIKey              string
	AgentName           string // unique agent name sent to the SaaS at registration
	EventBus            *event.Bus
	Logger              *slog.Logger
	WorkflowName        string
	WorkflowDescription string
	WorkflowTags        []string
	TriggerType         string // "http", "webhook", "schedule", "rabbitmq", ""
	StepsCount          int
	Version             string // tailflow binary version
	Revision            string // workflow revision
	FlushInterval       time.Duration
	HeartbeatInterval   time.Duration
}

// serverConfig is the configuration pushed back by the SaaS in response bodies.
type serverConfig struct {
	AgentID            string   `json:"agent_id,omitempty"`
	FlushIntervalS     *float64 `json:"flush_interval_s,omitempty"`
	HeartbeatIntervalS *float64 `json:"heartbeat_interval_s,omitempty"`
}

type Exporter struct {
	cfg               Config
	sessionID         string // generated locally at each startup
	client            *http.Client
	startAt           time.Time
	mu                sync.Mutex
	agentID           string // persistent, assigned by SaaS via /register response
	registered        bool
	activeExecutions  map[string]struct{}
	lastMetrics       map[string]any
	flushInterval     time.Duration
	heartbeatInterval time.Duration
	intervalChange    chan struct{} // signals batchLoop to reset its ticker
	wg                sync.WaitGroup
}

func New(cfg Config) *Exporter {
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 1 * time.Second
	}

	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 10 * time.Second
	}

	return &Exporter{
		cfg:               cfg,
		sessionID:         uuid.New().String(),
		client:            &http.Client{Timeout: 10 * time.Second},
		startAt:           time.Now(),
		activeExecutions:  make(map[string]struct{}),
		flushInterval:     cfg.FlushInterval,
		heartbeatInterval: cfg.HeartbeatInterval,
		intervalChange:    make(chan struct{}, 1),
	}
}

// Registration failures never block the agent — events are buffered and
// heartbeats are skipped until the SaaS assigns an agent_id.
func (e *Exporter) Start(ctx context.Context) {
	ch := e.cfg.EventBus.Subscribe(500)

	e.wg.Add(3)

	go func() { defer e.wg.Done(); e.register(ctx) }()
	go func() { defer e.wg.Done(); e.batchLoop(ctx, ch) }()
	go func() { defer e.wg.Done(); e.heartbeatLoop(ctx) }()
}

func (e *Exporter) Shutdown() {
	e.wg.Wait()
}

func (e *Exporter) isRegistered() (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.agentID, e.registered
}

// registerInitialBackoff and registerMaxBackoff control retry timing. Override in tests.
var (
	registerInitialBackoff = 1 * time.Second
	registerMaxBackoff     = 30 * time.Second
)

// tryRegister attempts one registration with the SaaS. On success it sets
// agentID/registered and returns true. On failure it logs and returns false.
func (e *Exporter) tryRegister(payload map[string]any, backoff time.Duration) bool {
	// Use background context so in-flight HTTP calls aren't aborted when the
	// parent ctx is canceled — registration must finish for finalFlush to work.
	resp, err := e.post(context.Background(), "/api/v1/agent/register", payload)
	if err != nil {
		e.cfg.Logger.Warn("export register failed, retrying", "error", err, "backoff", backoff)
		return false
	}

	var sc serverConfig

	jsonErr := json.Unmarshal(resp, &sc)
	if jsonErr != nil || sc.AgentID == "" {
		e.cfg.Logger.Warn("export register response missing agent_id, retrying")
		return false
	}

	e.mu.Lock()
	e.agentID = sc.AgentID
	e.registered = true
	e.mu.Unlock()

	e.cfg.Logger.Info("export registered", "agent_id", sc.AgentID)
	e.applyServerConfig(resp)

	return true
}

func (e *Exporter) register(ctx context.Context) {
	payload := map[string]any{
		"session_id":           e.sessionID,
		"agent_name":           e.cfg.AgentName,
		"workflow_name":        e.cfg.WorkflowName,
		"workflow_description": e.cfg.WorkflowDescription,
		"workflow_tags":        e.cfg.WorkflowTags,
		"revision":             e.cfg.Revision,
		"trigger_type":         e.cfg.TriggerType,
		"steps_count":          e.cfg.StepsCount,
		"version":              e.cfg.Version,
	}

	backoff := registerInitialBackoff
	maxBackoff := registerMaxBackoff

	for {
		if e.tryRegister(payload, backoff) { //nolint:contextcheck
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// maxBatchSize is the maximum number of events buffered before old events
// are dropped. This prevents unbounded memory growth when the SaaS is
// unreachable or rejecting payloads.
const maxBatchSize = 10_000

// flushChunkSize is the maximum number of events sent in a single /ingest
// request to stay well within typical body-size limits.
const flushChunkSize = 2_000

// flushBatch sends all buffered events to the SaaS in chunks.
// It clears the batch on success or drops failed chunks.
func (e *Exporter) flushBatch(flushCtx context.Context, batch *[]event.Event) {
	if len(*batch) == 0 {
		return
	}

	agentID, ok := e.isRegistered()
	if !ok {
		return // keep buffering until registered
	}

	// Send in chunks to avoid hitting server body-size limits.
	for len(*batch) > 0 {
		end := min(flushChunkSize, len(*batch))
		chunk := (*batch)[:end]

		payload := map[string]any{
			"agent_id":   agentID,
			"session_id": e.sessionID,
			"events":     chunk,
		}

		if _, err := e.post(flushCtx, "/api/v1/agent/ingest", payload); err != nil {
			e.cfg.Logger.Warn("export ingest failed", "error", err, "batch_size", len(*batch))
			// Drop the failed chunk to prevent infinite accumulation.
			*batch = (*batch)[end:]

			return
		}

		*batch = (*batch)[end:]
	}
}

// finalFlush drains any remaining events from the channel, waits briefly for
// registration if needed, then flushes the batch.
func (e *Exporter) finalFlush(ch <-chan event.Event, batch *[]event.Event) {
	// Drain any remaining events from the channel.
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				break
			}

			if ev.Type != event.Metrics {
				e.trackExecution(ev)
				*batch = append(*batch, ev)
			}

			continue
		default:
		}

		break
	}

	if len(*batch) == 0 {
		return
	}

	e.awaitRegistrationThenFlush(batch)
}

// awaitRegistrationThenFlush waits up to 5s for registration then flushes.
func (e *Exporter) awaitRegistrationThenFlush(batch *[]event.Event) {
	if _, ok := e.isRegistered(); !ok {
		deadline := time.After(5 * time.Second)
		tick := time.NewTicker(50 * time.Millisecond)

		defer tick.Stop()

		for {
			select {
			case <-deadline:
				e.cfg.Logger.Warn("export: shutdown timeout waiting for registration, events lost", "count", len(*batch))
				return
			case <-tick.C:
				if _, ok := e.isRegistered(); ok {
					goto ready
				}
			}
		}
	}

ready:
	e.flushBatch(context.Background(), batch)
}

// Events are buffered even before registration; they are only sent once
// the SaaS has assigned an agent_id.
func (e *Exporter) batchLoop(ctx context.Context, ch <-chan event.Event) {
	e.mu.Lock()
	ticker := time.NewTicker(e.flushInterval)
	e.mu.Unlock()

	defer ticker.Stop()

	var batch []event.Event

	for {
		select {
		case <-ctx.Done():
			e.finalFlush(ch, &batch) //nolint:contextcheck
			return
		case ev, ok := <-ch:
			if !ok {
				e.finalFlush(ch, &batch) //nolint:contextcheck
				return
			}

			if ev.Type == event.Metrics {
				e.mu.Lock()
				e.lastMetrics = ev.Data
				e.mu.Unlock()

				continue
			}

			e.trackExecution(ev)
			batch = append(batch, ev)
			// Cap buffer to prevent unbounded memory growth.
			if len(batch) > maxBatchSize {
				drop := len(batch) - maxBatchSize
				batch = batch[drop:]
				e.cfg.Logger.Warn("export buffer full, dropping oldest events", "dropped", drop)
			}
		case <-ticker.C:
			e.flushBatch(ctx, &batch)
		case <-e.intervalChange:
			e.mu.Lock()
			ticker.Reset(e.flushInterval)
			e.mu.Unlock()
		}
	}
}

func (e *Exporter) heartbeatLoop(ctx context.Context) {
	e.mu.Lock()
	ticker := time.NewTicker(e.heartbeatInterval)
	e.mu.Unlock()

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			agentID, ok := e.isRegistered()
			if !ok {
				continue // skip until registered
			}

			e.mu.Lock()
			active := len(e.activeExecutions)
			metrics := e.lastMetrics
			e.mu.Unlock()

			payload := map[string]any{
				"agent_id":          agentID,
				"session_id":        e.sessionID,
				"uptime_s":          int64(time.Since(e.startAt).Seconds()),
				"active_executions": active,
				"metrics":           metrics,
			}

			resp, err := e.post(ctx, "/api/v1/agent/heartbeat", payload)
			if err != nil {
				e.cfg.Logger.Warn("export heartbeat failed", "error", err)
				continue
			}

			if e.applyServerConfig(resp) {
				e.mu.Lock()
				ticker.Reset(e.heartbeatInterval)
				e.mu.Unlock()
			}
		}
	}
}

func (e *Exporter) applyServerConfig(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	var sc serverConfig

	err := json.Unmarshal(body, &sc)
	if err != nil {
		return false
	}

	changed := false
	flushChanged := false

	e.mu.Lock()

	if sc.HeartbeatIntervalS != nil {
		d := time.Duration(*sc.HeartbeatIntervalS * float64(time.Second))
		if d >= 1*time.Second && d != e.heartbeatInterval {
			e.cfg.Logger.Info("export config: heartbeat interval updated", "interval", d)
			e.heartbeatInterval = d
			changed = true
		}
	}

	if sc.FlushIntervalS != nil {
		d := time.Duration(*sc.FlushIntervalS * float64(time.Second))
		if d >= 100*time.Millisecond && d != e.flushInterval {
			e.cfg.Logger.Info("export config: flush interval updated", "interval", d)
			e.flushInterval = d
			flushChanged = true
		}
	}

	e.mu.Unlock()

	if flushChanged {
		select {
		case e.intervalChange <- struct{}{}:
		default:
		}
	}

	return changed
}

func (e *Exporter) trackExecution(ev event.Event) {
	switch ev.Type {
	case event.WorkflowStarted:
		e.mu.Lock()
		e.activeExecutions[ev.ExecutionID] = struct{}{}
		e.mu.Unlock()
	case event.WorkflowCompleted:
		e.mu.Lock()
		delete(e.activeExecutions, ev.ExecutionID)
		e.mu.Unlock()
	case event.StepStarted, event.StepCompleted, event.StepFailed, event.StepSkipped,
		event.StepLog, event.StepWaiting, event.StepInput, event.StepOutput,
		event.StepGoto, event.Metrics:
	}
}

func (e *Exporter) post(ctx context.Context, path string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.ExportURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return respBody, nil
}
