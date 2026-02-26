<p align="center">
  <h1 align="center">TailFlow</h1>
  <p align="center">
    <strong>Define your backend logic in YAML. TailFlow runs it.</strong>
  </p>
  <p align="center">
    One binary. Zero infrastructure. 34 built-in actions covering HTTP, SQL, queues, locks, scripting, and sync-over-async webhooks.
  </p>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#use-cases">Use Cases</a> &bull;
  <a href="#actions">Actions</a> &bull;
  <a href="#examples">Examples</a> &bull;
  <a href="#saas-export">SaaS Export</a> &bull;
  <a href="#architecture">Architecture</a>
</p>

---

## Why TailFlow?

Building backends means writing the same patterns over and over: validate input, call an API, wait for a callback, retry on failure, handle transactions, prevent race conditions...

TailFlow lets you express that logic as a **YAML workflow** and handles the rest.

| Problem | Without TailFlow | With TailFlow |
|---------|------------------|---------------|
| Payment flow with webhook callback | 200+ lines of Go/Node/Python, custom state machine, webhook server | 30 lines of YAML |
| API endpoint with validation + DB transaction | Controller, service, repository layers, error handling | One YAML file |
| Retry logic with exponential backoff | Custom retry wrapper, error classification | `retry: { max: 3 }` |
| Prevent double-spend on concurrent requests | Redis lock implementation, cleanup logic | `action: lock` |
| Shovel messages between RabbitMQ queues | Custom consumer/publisher, ack logic, error handling | `action: rabbitmq.shovel` |

### How it compares

| | TailFlow | Temporal | Airflow | n8n |
|---|----------|----------|---------|-----|
| Infrastructure needed | **None** (single binary) | Cassandra/PostgreSQL cluster | PostgreSQL + Redis + workers | PostgreSQL + Redis |
| Language | YAML + JS expressions | Go/Java/Python SDK | Python DAGs | Visual no-code |
| HTTP triggers | Built-in | Requires custom server | Not designed for it | Limited |
| Sync-over-async (wait for webhook) | Built-in | Manual signal handling | Not supported | Limited |
| SQL transactions | Built-in (begin/commit/rollback) | Manual | Not applicable | Via plugins |
| Web UI | Embedded (zero setup) | Separate deployment | Separate deployment | Built-in |
| SaaS monitoring | Built-in agent exporter | N/A | N/A | N/A |
| Best for | API orchestration, event-driven backends | Long-running distributed workflows | Data pipelines | Simple automations |

---

## Quick Start

### Install

```bash
# From source
go install github.com/tailflow/tailflow/cmd/tailflow@latest

# Or clone and build
git clone https://github.com/tailflow/tailflow.git
cd tailflow && make build
```

### Docker

```bash
# Build the image
make docker-build

# Or directly
docker build -f build/package/Dockerfile -t tailflow:dev .
```

### Run a workflow

```bash
# CLI mode - run and exit
tailflow run examples/hello.yaml -p name=World

# Server mode - embedded UI + API + triggers
tailflow serve examples/payment-sync.yaml --port 8080

# Self-hosted mode - enables exec, js, file.* actions
tailflow serve --selfhosted examples/ping.yaml

# With SaaS monitoring
tailflow serve --selfhosted \
  --exporter-url https://saas.example.com \
  --exporter-key my-key \
  --exporter-name my-agent \
  examples/ping.yaml
```

### Your first workflow

```yaml
version: "2.0"
name: "hello-world"
description: "A simple hello world workflow"
revision: "1.0.0"

params:
  - name: name
    type: string
    default: "World"

steps:
  - id: greet
    action: log
    title: "Say hello"
    config:
      message: "Hello, {{ params.name }}!"
```

---

## Use Cases

### 1. Payment Orchestration

Call a payment provider, **wait for the webhook callback**, then respond to the client - all in one synchronous HTTP request.

```yaml
version: "2.0"
name: "payment-sync"
trigger:
  http:
    method: POST
    path: /pay

steps:
  - id: validate
    action: js
    config:
      script: |
        if (!trigger.body.amount) throw new Error("amount required");
        return { order_id: "ord_" + Date.now(), amount: trigger.body.amount };

  - id: call-provider
    action: http
    depends_on: [validate]
    config:
      method: POST
      url: "https://provider.com/charge"
      body:
        amount: "{{ steps.validate.output.amount }}"
        callback_url: "{{ env.BASE_URL }}/api/wait/{{ execution.id }}/provider-callback"

  - id: wait-callback
    action: wait.webhook
    depends_on: [call-provider]
    config:
      path: /provider-callback
      timeout: 5m

  - id: respond
    action: response
    depends_on: [wait-callback]
    config:
      status: 200
      body:
        paid: true
        order_id: "{{ steps.validate.output.order_id }}"
```

**What happens:** Client POSTs to `/pay` -> workflow calls provider -> **pauses and waits** for provider to call back -> responds to the original client request. Zero polling, zero state machine.

### 2. API Backend with SQL Transactions

Build a complete API endpoint with ACID guarantees:

```yaml
version: "2.0"
name: "create-order"
trigger:
  http:
    method: POST
    path: /api/public/orders

steps:
  - id: begin
    action: sql.begin
    config:
      dsn: "{{ env.DATABASE_URL }}"
      name: "order_tx"

  - id: insert_order
    action: sql.exec
    depends_on: [begin]
    config:
      tx: "order_tx"
      query: "INSERT INTO orders (customer_id, total) VALUES ($1, $2) RETURNING id"
      params: ["{{ trigger.body.customer_id }}", "{{ trigger.body.total }}"]
    on_error:
      goto: rollback

  - id: commit
    action: sql.commit
    depends_on: [insert_order]
    config:
      name: "order_tx"

  - id: respond
    action: response
    depends_on: [commit]
    config:
      status: 201
      body:
        order_id: "{{ steps.insert_order.output.last_insert_id }}"

  - id: rollback
    action: sql.rollback
    config:
      name: "order_tx"
```

### 3. Race Condition Prevention

Distributed locks prevent concurrent operations from conflicting:

```yaml
steps:
  - id: acquire
    action: lock
    config:
      key: "payment-{{ trigger.body.order_id }}"
      timeout: "30s"

  - id: process
    action: http
    depends_on: [acquire]
    config:
      method: POST
      url: "https://bank.com/transfer"

  - id: release
    action: unlock
    depends_on: [process]
    config:
      key: "payment-{{ trigger.body.order_id }}"
```

### 4. Scheduled Monitoring

Cron-triggered workflows for periodic tasks:

```yaml
version: "2.0"
name: "healthcheck"
revision: "1.0.0"
trigger:
  schedule:
    cron: "*/5 * * * *"

steps:
  - id: check
    action: http
    config:
      method: GET
      url: "https://myapp.com/health"
      timeout: 10s
    retry:
      max: 3

  - id: alert
    action: http
    depends_on: [check]
    when: "steps.check.output.status != 200"
    config:
      method: POST
      url: "{{ env.SLACK_WEBHOOK }}"
      body:
        text: "Healthcheck failed!"
```

### 5. RabbitMQ Event Processing

Process messages from a queue with automatic ack/nack:

```yaml
version: "2.0"
name: "order-processor"
trigger:
  rabbitmq:
    url: "{{ env.RABBITMQ_URL }}"
    queue: "orders"
    prefetch: 5
    ack_on_success: true

steps:
  - id: process
    action: http
    config:
      method: POST
      url: "https://api.internal/process-order"
      body: "{{ trigger.body }}"
```

### 6. RabbitMQ Shovel

Forward messages between queues with per-message safety via native AMQP:

```yaml
steps:
  - id: shovel
    action: rabbitmq.shovel
    config:
      source_url: "{{ env.RABBITMQ_SOURCE }}"
      source_queue: "incoming"
      dest_url: "{{ env.RABBITMQ_DEST }}"
      dest_exchange: "processed"
      dest_routing_key: "orders"
      count: 100
      timeout: 30s
```

### 7. DevOps Automation

Multi-step deployment with conditional rollback:

```yaml
steps:
  - id: build
    action: exec
    config:
      command: "docker build -t myapp:{{ params.version }} ."

  - id: test
    action: exec
    depends_on: [build]
    config:
      command: "docker run myapp:{{ params.version }} npm test"
    retry:
      max: 2

  - id: deploy
    action: exec
    depends_on: [test]
    config:
      command: "kubectl set image deployment/myapp app=myapp:{{ params.version }}"
```

---

## Actions

TailFlow ships with **34 built-in actions**:

| Category | Actions | Description |
|----------|---------|-------------|
| **Core** | `set`, `log`, `response` | Variables, logging, HTTP responses |
| **Scripting** | `js`, `template` | JavaScript (ES6 via Goja) execution, template rendering |
| **Control** | `condition`, `loop`, `delay`, `schedule` | Branching, iteration, waiting, deferred execution |
| **HTTP** | `http` | REST calls with full control (headers, auth, retries) |
| **Database** | `sql.query`, `sql.exec`, `sql.begin`, `sql.commit`, `sql.rollback` | Full SQL with ACID transactions |
| **Data** | `json.decode`, `json.encode`, `validate` | Data transformation and validation |
| **Files** | `file.read`, `file.write` | Filesystem operations (self-hosted only) |
| **Strings** | `string.match_all`, `string.replace` | Regex matching and replacement |
| **Math** | `math` | Arithmetic operations |
| **Arrays** | `array.sort`, `array.filter`, `array.map`, `array.uniq`, `array.pick`, `array.concat` | Sorting, filtering, mapping, deduplication, selection, concatenation |
| **Objects** | `object` | Object creation and manipulation |
| **Hash** | `hash` | Hashing (MD5, SHA256, etc.) |
| **KV Store** | `kv.get`, `kv.set`, `kv.delete` | Key-value storage (in-memory or Redis) |
| **Async** | `wait.webhook`, `wait.rabbitmq` | Pause workflow until external event |
| **Concurrency** | `lock`, `unlock` | Distributed locking |
| **Messaging** | `rabbitmq.shovel` | RabbitMQ message forwarding via native AMQP |
| **System** | `exec` | Shell command execution (self-hosted only) |

### Step features

```yaml
- id: my-step
  action: http
  title: "Call external API"
  depends_on: [previous-step]       # DAG dependencies
  when: "vars.should_call == true"  # Conditional execution
  timeout: 30s                      # Per-step timeout
  error_policy: continue            # stop (default), continue, or ignore
  retry:
    max: 3                          # Retry with exponential backoff
    delay: 2s                       # Initial retry delay
  config:
    url: "https://api.example.com"
  on_error:                         # Error handler steps
    - id: log-error
      action: log
      config:
        message: "API call failed: {{ steps.my-step.error }}"
  goto:                             # Conditional loops
    target: my-step
    when: "steps.my-step.output.status == 'retry'"
    max_iterations: 5
```

---

## Triggers

TailFlow supports 4 trigger types:

| Trigger | Description | Example |
|---------|-------------|---------|
| **HTTP** | Expose workflow as REST endpoint | `trigger: { http: { method: POST, path: /api, async: false } }` |
| **Webhook** | Listen for incoming webhooks with optional HMAC validation and filtering | `trigger: { webhook: { path: /hook, secret: "...", filter: "..." } }` |
| **Schedule** | Cron-based execution | `trigger: { schedule: { cron: "*/5 * * * *" } }` |
| **RabbitMQ** | Process messages from a queue | `trigger: { rabbitmq: { url: "...", queue: "orders" } }` |

---

## Examples

The [`examples/`](./examples) directory contains ready-to-run workflows:

| Example | Description | Key Features |
|---------|-------------|--------------|
| [`hello.yaml`](./examples/hello.yaml) | Hello world | Parameters, variables |
| [`ping.yaml`](./examples/ping.yaml) | Ping a host | Exec, streaming output |
| [`api-trigger.yaml`](./examples/api-trigger.yaml) | REST API endpoint | HTTP trigger, JS validation, response |
| [`payment-sync.yaml`](./examples/payment-sync.yaml) | Synchronous payment | Wait for webhook, sync-over-async |
| [`payment-async.yaml`](./examples/payment-async.yaml) | Async payment | Async HTTP trigger, 202 response |
| [`payment-webhook.yaml`](./examples/payment-webhook.yaml) | Payment with callback | Webhook trigger, async processing |
| [`sql-transaction.yaml`](./examples/sql-transaction.yaml) | SQL transactions | Begin, insert, commit, rollback on error |
| [`sql-users.yaml`](./examples/sql-users.yaml) | User CRUD | SQL queries, HTTP response |
| [`lock-payment.yaml`](./examples/lock-payment.yaml) | Race condition prevention | Distributed locks |
| [`js-workflow.yaml`](./examples/js-workflow.yaml) | JavaScript scripting | ES6 via Goja, computed values |
| [`loop.yaml`](./examples/loop.yaml) | List iteration | Loop action with pipeline |
| [`goto.yaml`](./examples/goto.yaml) | Conditional loops | Goto with max iterations |
| [`validate-input.yaml`](./examples/validate-input.yaml) | Schema validation | Input validation rules |
| [`parallel-exec.yaml`](./examples/parallel-exec.yaml) | Parallel execution | DAG parallelism |
| [`error-handling-demo.yaml`](./examples/error-handling-demo.yaml) | Error handling | on_error, error_policy, retry |
| [`healthcheck.yaml`](./examples/healthcheck.yaml) | Health monitoring | Cron trigger, HTTP checks, retry |
| [`order-rabbitmq.yaml`](./examples/order-rabbitmq.yaml) | RabbitMQ processing | Queue consumer, wait.rabbitmq |
| [`deploy.yaml`](./examples/deploy.yaml) | Deployment pipeline | Multi-step exec |
| [`rabbitmq-shovel.yaml`](./examples/rabbitmq-shovel.yaml) | RabbitMQ shovel | Native AMQP message forwarding |
| [`data-transform.yaml`](./examples/data-transform.yaml) | Data transformation | json.encode/decode, template, object, math |
| [`array-operations.yaml`](./examples/array-operations.yaml) | Array manipulation | sort, filter, map, uniq, pick, concat |
| [`string-hash.yaml`](./examples/string-hash.yaml) | String & hashing | string.match_all, string.replace, hash |
| [`kv-store.yaml`](./examples/kv-store.yaml) | Key-value store | kv.get, kv.set, kv.delete, condition |
| [`file-operations.yaml`](./examples/file-operations.yaml) | File read/write | file.read, file.write (self-hosted) |
| [`delay-schedule.yaml`](./examples/delay-schedule.yaml) | Timed workflows | delay, schedule |

---

## SaaS Export

TailFlow agents can push events, heartbeats, and system metrics to a central SaaS platform for distributed workflow monitoring.

```
┌──────────────────┐         HTTPS          ┌──────────────────┐
│   TailFlow Agent │ ─────────────────────> │   SaaS Platform  │
│   (self-hosted)  │                        │   (central)      │
│                  │  /register             │                  │
│  - Runs workflow │  /ingest (events)      │  - Dashboard     │
│  - Local UI      │  /heartbeat (metrics)  │  - Alerting      │
│  - Export events │ <───────────────────── │  - History       │
│                  │  agent_id, config      │                  │
└──────────────────┘                        └──────────────────┘
```

### Enable export

```bash
tailflow serve --selfhosted \
  --exporter-url https://saas.example.com \
  --exporter-key my-api-key \
  --exporter-name my-agent \
  examples/ping.yaml
```

### How it works

1. **Registration** - Agent sends workflow metadata to `/register`. The SaaS responds with an `agent_id` and optional config overrides.
2. **Event ingestion** - Workflow events (step started/completed/failed, logs) are batched and flushed to `/ingest` every 1s.
3. **Heartbeat** - Agent sends uptime, active execution count, and system metrics (CPU, memory, goroutines) to `/heartbeat` every 10s.

### Protocol

**`POST /register`** (agent -> SaaS)
```json
{
  "session_id": "uuid",
  "workflow_name": "ping",
  "workflow_description": "Ping a host",
  "workflow_tags": ["example", "network"],
  "revision": "1.0.0",
  "trigger_type": "schedule",
  "steps_count": 5,
  "version": "1.0.0"
}
```

**Response** (SaaS -> agent)
```json
{
  "agent_id": "uuid-assigned-by-saas",
  "heartbeat_interval_s": 5,
  "flush_interval_s": 1
}
```

**`POST /ingest`** (every flush interval)
```json
{
  "agent_id": "uuid",
  "session_id": "uuid",
  "events": [{"type": "step.started", "timestamp": "...", "step_id": "ping", ...}]
}
```

**`POST /heartbeat`** (every heartbeat interval)
```json
{
  "agent_id": "uuid",
  "session_id": "uuid",
  "uptime_s": 3600,
  "active_executions": 2,
  "metrics": {
    "cpu_percent": 12.5,
    "rss_kb": 45000,
    "goroutines": 8,
    "heap_mb": 3.2
  }
}
```

### Design

- **Non-blocking** - Export failures never affect workflow execution. Events are buffered until registration succeeds.
- **Server-driven config** - The SaaS pushes interval overrides via `/register` and `/heartbeat` responses (e.g., free plan = 30s heartbeat, pro = 5s).
- **Session tracking** - `session_id` (generated per startup) lets the SaaS detect agent restarts. `agent_id` (persistent, SaaS-assigned) identifies the deployment.

### Test locally

```bash
# Terminal 1: start the debug export server
go run ./examples/debug-export-server/

# Terminal 2: start agent with export
tailflow serve --selfhosted --exporter-url http://localhost:9090 --exporter-key test examples/ping.yaml
```

Or with Docker:

```bash
cd personal_examples
docker compose -f docker-compose.export.yaml up --build
```

---

## Architecture

```
                    ┌─────────────────────────────────┐
                    │         YAML Workflow            │
                    │    (parser + validator)          │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │       DAG Engine                 │
                    │  (topological sort, parallel     │
                    │   execution, retries, goto)      │
                    └──────────┬──────────────────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                    │
  ┌───────▼──────┐   ┌────────▼───────┐   ┌───────▼──────┐
  │   Actions    │   │  Event Bus     │   │   Services   │
  │  (34 built   │   │  (real-time    │   │ (DB pool,    │
  │   in)        │   │   SSE stream)  │   │  KV store,   │
  │              │   │                │   │  locks, wait)│
  └──────────────┘   └──┬─────┬──────┘   └──────────────┘
                        │     │
               ┌────────▼┐   ┌▼───────────┐
               │  Web UI │   │  SaaS      │
               │(embedded│   │  Exporter  │
               │  SPA)   │   │ (optional) │
               └─────────┘   └────────────┘
```

### Key design decisions

- **Single binary**: The web UI is embedded. No Node.js, no Docker, no external services required.
- **DAG execution**: Steps run in parallel when dependencies allow. Semaphore limits concurrency to CPU count.
- **Event-driven**: Every step emits events streamed via SSE to the UI in real-time.
- **Non-blocking export**: The SaaS exporter runs independently. Failures never affect local execution.
- **Extensible services**: Interfaces for `Locker`, `DBPool`, `KVStore` allow plugging in Redis/PostgreSQL for multi-instance deployments.

---

## CLI Reference

### `tailflow run`

Execute a workflow in CLI mode (run and exit).

```bash
tailflow run <workflow.yaml> [flags]
```

| Flag | Description |
|------|-------------|
| `--param, -p` | Parameters as key=value (repeatable) |
| `--data, -d` | Trigger body as JSON string |

### `tailflow serve`

Start the web server with embedded UI, API, and triggers.

```bash
tailflow serve <workflow.yaml> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port, -P` | `8080` | Server port |
| `--max-executions` | `100` | Max executions kept in memory |
| `--selfhosted` | `false` | Enable exec, js, file.* actions |

### `tailflow validate`

Validate a workflow file without executing it.

```bash
tailflow validate <workflow.yaml>
```

### Global flags

| Flag | Env variable | Description |
|------|-------------|-------------|
| `--no-color` | | Disable colour output |
| `--exporter-url` | `TAILFLOW_EXPORTER_URL` | SaaS endpoint URL for event export |
| `--exporter-key` | `TAILFLOW_EXPORTER_KEY` | API key for SaaS authentication |
| `--exporter-name` | `TAILFLOW_EXPORTER_NAME` | Unique agent name |

### API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/workflow` | Get workflow definition |
| `GET` | `/api/workflow/graph` | Get workflow DAG graph |
| `GET` | `/api/workflow/activity` | Get execution activity history |
| `GET` | `/api/workflow/steps/{id}` | Get step detail |
| `POST` | `/api/workflow/run` | Trigger workflow execution |
| `POST` | `/api/workflow/validate` | Validate workflow |
| `GET` | `/api/executions` | List all executions |
| `GET` | `/api/executions/{id}` | Get execution details |
| `POST` | `/api/executions/{id}/cancel` | Cancel running execution |
| `GET` | `/api/executions/{id}/events` | SSE stream for execution |
| `GET` | `/api/events` | Global SSE stream |
| `GET` | `/api/metrics` | Workflow metrics |

---

## Workflow DSL Reference

```yaml
version: "2.0"                    # Required - schema version
name: "my-workflow"               # Workflow name
description: "What it does"       # Optional
revision: "1.0.0"                 # Optional - workflow version (semver, int, git hash...)
tags: ["api", "payments"]         # Optional - for categorization
author: "team-name"               # Optional

params:                           # Input parameters
  - name: user_id
    type: string                  # string, int, float, bool
    required: true
    pattern: "[a-zA-Z0-9_-]+"    # Optional regex validation (security)
  - name: dry_run
    type: bool
    default: false

env:                              # Environment variables (template interpolation)
  DATABASE_URL: "{{ env.DB_URL }}"

trigger:                          # Trigger (server mode only)
  http:                           # HTTP endpoint
    method: POST
    path: /my-endpoint
    async: false                  # true = return 202 immediately
  # OR
  webhook:                        # Webhook listener
    path: /hook
    secret: "my-secret"           # Optional HMAC validation
    filter: "body.type == 'order'" # Optional JS filter expression
  # OR
  schedule:                       # Cron trigger
    cron: "*/5 * * * *"
  # OR
  rabbitmq:                       # RabbitMQ consumer
    url: "{{ env.RABBITMQ_URL }}"
    queue: "orders"
    prefetch: 10
    ack_on_success: true

on_error:                         # Workflow-level error handler
  - id: notify-failure
    action: http
    config:
      method: POST
      url: "{{ env.SLACK_WEBHOOK }}"
      body:
        text: "Workflow failed: {{ error }}"

steps:                            # Workflow steps (DAG)
  - id: step-id                   # Unique step identifier
    action: action-name           # One of 34 built-in actions
    title: "Human-readable title"
    depends_on: [other-step]      # DAG dependencies
    when: "expression"            # Skip condition
    timeout: 30s                  # Step timeout
    error_policy: continue        # stop (default), continue, or ignore
    retry:
      max: 3                      # Retry count
      delay: 2s                   # Initial retry delay
    config:                       # Action-specific configuration
      key: "value"
    on_error:                     # Error handler sub-steps
      - id: handle-error
        action: log
    goto:                         # Conditional jump (loops)
      target: step-id
      when: "expression"
      max_iterations: 10
```

### Sensitive Fields

Declare sensitive key names at the workflow top-level to automatically mask their values in all external outputs (SSE events, SaaS exporter, REST API). Internal step-to-step resolution keeps the real values.

```yaml
version: "2.0"
name: "payment"

sensitive:
  - token
  - refresh_token
  - card_number
  - api_key
  - password

params:
  - name: api_key
    type: string

steps:
  - id: auth
    action: http
    config:
      url: "https://api.example.com/auth"
```

Any key matching a name in the `sensitive` list is replaced with `[SENSITIVE]` **recursively** — at any depth, in params, step outputs, and configs — across all external channels. Steps still receive the real values via template resolution (`{{ steps.auth.output.token }}`).

### Template expressions

Access runtime data in any config value:

| Expression | Description |
|------------|-------------|
| `{{ params.name }}` | Input parameter |
| `{{ env.DATABASE_URL }}` | Environment variable |
| `{{ vars.counter }}` | Workflow variable (set by `set` action) |
| `{{ steps.step_id.output }}` | Step output |
| `{{ steps.step_id.output.field }}` | Nested step output |
| `{{ trigger.body }}` | HTTP trigger request body |
| `{{ trigger.headers }}` | HTTP trigger headers |
| `{{ trigger.query }}` | HTTP trigger query params |
| `{{ execution.id }}` | Current execution ID |

### Built-in functions

| Function | Description | Example |
|----------|-------------|---------|
| `uuid()` | Generate a UUID v4 | `{{ uuid() }}` |
| `now()` | Current time (RFC3339, UTC) | `{{ now() }}` |
| `now("tz")` | Current time in timezone | `{{ now("Europe/Paris") }}` |
| `tz(date, "tz")` | Convert date to timezone | `{{ tz(now(), "Asia/Tokyo") }}` |
| `formatDate(date, fmt)` | Format a date | `{{ formatDate(now(), "date") }}` |
| `addDate(date, dur)` | Add duration to date | `{{ addDate(now(), "2h") }}` |
| `diffDate(d1, d2)` | Difference in seconds | `{{ diffDate(d1, d2) }}` |
| `unixTime()` | Current unix timestamp | `{{ unixTime() }}` |
| `flag(name, val)` | CLI flag with value (empty if val is empty) | `{{ flag("--id", params.id) }}` |
| `bflag(name, bool)` | Boolean CLI flag (empty if false) | `{{ bflag("-v", params.verbose) }}` |

**CLI flag helpers** - Build dynamic commands with optional arguments:

```yaml
params:
  - name: id
    type: string
    default: ""
  - name: verbose
    type: bool
    default: false

steps:
  - id: run
    action: exec
    config:
      command: "mytool {{ bflag('-v', params.verbose) }} {{ flag('--id', params.id) }}"
      # {"verbose": true, "id": "42"} → "mytool -v --id 42"
      # {}                             → "mytool"
```

### Parameter validation

Use the `pattern` field to validate parameters with a regex before execution. This prevents injection attacks when parameters are interpolated into commands:

```yaml
params:
  - name: host
    type: string
    required: true
    default: "google.fr"
    pattern: "[a-zA-Z0-9._-]+"   # blocks "google.fr && rm -rf /"
```

If a value doesn't match the pattern, the workflow fails immediately with a clear error message.

---

## Project Structure

```
tailflow/
├── cmd/tailflow/          # CLI entry point
├── internal/
│   ├── action/            # 34 action implementations
│   ├── engine/            # DAG execution engine
│   ├── event/             # Event bus (SSE streaming)
│   ├── parser/            # YAML workflow parser
│   ├── runtime/           # Expression evaluation, services
│   ├── server/            # HTTP server, API handlers
│   ├── trigger/           # Trigger implementations
│   ├── store/             # Execution storage
│   ├── lock/              # Distributed locking
│   └── metrics/           # Metrics collection
├── pkg/                   # Public library packages
├── web/frontend/          # Vue.js embedded UI
├── examples/              # Ready-to-run workflow examples
├── scripts/               # Build and utility scripts
└── build/package/         # Dockerfile
```

## Development

```bash
# Build (with embedded frontend)
make build

# Build (Go only, faster)
make build-quick

# Run tests
make test

# Run tests with coverage
make test-coverage

# Lint
make lint

# Generate mocks
make gen-mocks

# Build frontend (requires Node.js)
cd web/frontend && npm install && npm run build

# Build Docker image
make docker-build
```

---

## Built with AI

TailFlow is a solo project where I wear the architect and tech lead hat: I define the vision, design the architecture, write the specs, and decide every technical trade-off.

AI (Claude) acts as my team. I give it precise tasks, constraints, and acceptance criteria — the same way I would brief engineers on a team. It writes code, tests, docs, and refactors under my direction. Every line is reviewed and validated by me before it ships.

This workflow lets a single person ship what would normally require a small team, without compromising on code quality or architecture. The decisions are mine. The execution is accelerated.

**In practice:** I architect, AI implements, I review. That's the loop.

---

## License

[Apache 2.0](LICENSE)
