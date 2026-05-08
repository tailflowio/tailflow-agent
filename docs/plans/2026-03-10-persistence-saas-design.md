# Persistence & SaaS Orchestration Design

## Context

Tailflow agent is currently fully stateless — all execution state lives in an in-memory ring buffer (`MemoryExecutionStore`). A restart loses everything. For workflow engine use cases (onboarding flows, multi-step processes, long-running workflows), this is a blocker.

Additionally, the SaaS currently runs on MariaDB + ClickHouse (dual database). This design consolidates everything onto **TiDB** (single database, MySQL-compatible, with TiFlash for OLAP).

## Vision

The agent stays **stateless and lightweight**. The SaaS becomes the **persistent brain**:

- Without SaaS: agent works exactly as today (in-memory, ephemeral)
- With SaaS: executions are persisted, recoverable, queryable, and groupable

This creates a natural open-core model where the SaaS provides real production value.

## Architecture

```
┌─────────────┐  push execution state   ┌──────────────────┐
│    Agent    │ ──────────────────────▶ │      SaaS        │
│ (stateless) │ ◀────────────────────── │     (TiDB)       │
└─────────────┘  pull recovery state    │  + TiFlash OLAP  │
                                        └──────────────────┘
                                                │
                                        ┌───────┴───────┐
                                        │ tailflow-shared│
                                        │  (Go + TS)    │
                                        └───────────────┘
```

### Three repos

| Repo | Role |
|------|------|
| `tailflow-agent` | Stateless worker: executes workflows, pushes state, pulls recovery |
| `tailflow-saas` | Persistent brain: TiDB for all storage, serves dashboard |
| `tailflow-shared` | Shared types: event types, data payloads, TS types for frontend |

### Database migration: MariaDB + ClickHouse → TiDB

| Before | After |
|--------|-------|
| MariaDB (OLTP) | TiDB (OLTP, MySQL-compatible) |
| ClickHouse (OLAP) | TiFlash (columnar replica on same TiDB cluster) |
| 2 Go drivers | 1 Go driver (`go-sql-driver/mysql`) |
| 2 migration paths | 1 migration path |
| 2 repository layers | 1 repository layer |

TiDB is MySQL wire-compatible. Existing MariaDB schemas migrate as-is. ClickHouse-specific features (`LowCardinality`, `MergeTree`, `PARTITION BY`, native `TTL`) are replaced with:
- Standard SQL tables with `TiFlash REPLICA` for analytics
- Cron job for TTL: `DELETE FROM events WHERE event_timestamp < NOW() - INTERVAL 90 DAY`
- Standard indexes instead of ClickHouse ordering keys

### Agent behavior

| Mode | Behavior |
|------|----------|
| Standalone (no SaaS) | Unchanged. Memory store, ring buffer, ephemeral. |
| Connected to SaaS | Push execution state via enriched exporter. Pull state on recovery. |

## Phase 1 — DB Migration + Persistence + Recovery + Idempotency

### 1.1 TiDB Migration (SaaS)

Migrate all existing MariaDB tables to TiDB (schema identical, MySQL-compatible).

Migrate ClickHouse tables to TiDB with TiFlash replicas:

```sql
CREATE TABLE IF NOT EXISTS events (
    id              BIGINT AUTO_INCREMENT PRIMARY KEY,
    organization_id CHAR(36) NOT NULL,
    agent_id        CHAR(36) NOT NULL,
    session_id      VARCHAR(255) NOT NULL,
    execution_id    VARCHAR(255) NOT NULL DEFAULT '',
    event_type      VARCHAR(50) NOT NULL,
    step_id         VARCHAR(255) NOT NULL DEFAULT '',
    message         TEXT,
    data            JSON,
    event_timestamp DATETIME(3) NOT NULL,
    seq             BIGINT NOT NULL DEFAULT 0,
    ingested_at     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    INDEX idx_org_agent_exec_ts (organization_id, agent_id, execution_id, event_timestamp),
    INDEX idx_org_ts (organization_id, event_timestamp),
    INDEX idx_execution (execution_id, event_timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE events SET TIFLASH REPLICA 1;

CREATE TABLE IF NOT EXISTS heartbeats (
    id                BIGINT AUTO_INCREMENT PRIMARY KEY,
    organization_id   CHAR(36) NOT NULL,
    agent_id          CHAR(36) NOT NULL,
    session_id        VARCHAR(255) NOT NULL,
    uptime_s          BIGINT NOT NULL DEFAULT 0,
    active_executions INT NOT NULL DEFAULT 0,
    cpu_percent       DOUBLE NOT NULL DEFAULT 0,
    memory_bytes      BIGINT NOT NULL DEFAULT 0,
    net_rx_bytes      BIGINT NOT NULL DEFAULT 0,
    net_tx_bytes      BIGINT NOT NULL DEFAULT 0,
    recorded_at       DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    INDEX idx_org_agent_ts (organization_id, agent_id, recorded_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE heartbeats SET TIFLASH REPLICA 1;
```

### 1.2 New execution tables (TiDB)

```sql
CREATE TABLE IF NOT EXISTS executions (
    id                CHAR(36) NOT NULL PRIMARY KEY,
    organization_id   CHAR(36) NOT NULL,
    agent_session_id  CHAR(36) NOT NULL,
    workflow_name     VARCHAR(255) NOT NULL,
    status            VARCHAR(50) NOT NULL,
    params            JSON,
    error_message     TEXT,
    idempotency_key   VARCHAR(255),
    started_at        DATETIME(3) NOT NULL,
    finished_at       DATETIME(3),
    UNIQUE KEY uq_idempotency (organization_id, workflow_name, idempotency_key),
    INDEX idx_org_status (organization_id, status),
    INDEX idx_org_workflow (organization_id, workflow_name),
    INDEX idx_agent_status (agent_session_id, status),
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (agent_session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS execution_steps (
    execution_id  CHAR(36) NOT NULL,
    step_id       VARCHAR(255) NOT NULL,
    status        VARCHAR(50) NOT NULL,
    on_recovery   VARCHAR(10) NOT NULL DEFAULT 'retry',
    input_data    JSON,
    output_data   JSON,
    error_message TEXT,
    error_code    VARCHAR(50),
    started_at    DATETIME(3),
    finished_at   DATETIME(3),
    PRIMARY KEY (execution_id, step_id),
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS execution_group_params (
    execution_id  CHAR(36) NOT NULL,
    param_name    VARCHAR(255) NOT NULL,
    param_value   VARCHAR(255) NOT NULL,
    INDEX idx_lookup (param_name, param_value),
    INDEX idx_execution (execution_id),
    FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

### 1.3 Enriched Export (Agent → SaaS)

The existing `internal/export/` already pushes events to the SaaS. Enrich it to push **full execution state**:

- On execution start: push execution + params + workflow metadata
- On step complete: push step result
- On execution complete: push final status
- On step waiting: push waiting state

New shared event type `ExecutionState` in `tailflow-shared/pkg/event/types.go`.

The agent treats the SaaS as fire-and-forget. SaaS unreachable = agent continues normally.

### 1.4 Recovery (SaaS → Agent)

On agent startup, if connected to SaaS:

1. Agent calls `GET /api/v1/agent/recovery?agent_id={id}`
2. SaaS queries TiDB: `SELECT * FROM executions WHERE agent_session_id = ? AND status IN ('running', 'waiting')`
3. Agent rebuilds ExecutionContext from persisted state
4. Agent resumes DAG — skips steps with status `success`, applies `on_recovery` for steps with status `running`

Recovery is best-effort. SaaS unreachable at startup = agent starts fresh.

### 1.5 Idempotency (two levels)

#### Workflow-level

```yaml
trigger:
  http:
    method: POST
    path: /api/onboard
    idempotency_key: "{{ trigger.body.customer_id }}"
```

Agent sends key to SaaS → `INSERT ... ON DUPLICATE KEY` on TiDB `uq_idempotency` index → dedup.

#### Step-level (on_recovery)

| Strategy | Behavior | Use case |
|----------|----------|----------|
| `retry` (default) | Re-execute | Idempotent actions (PUT, upsert) |
| `skip` | Mark success | Side-effect actions (email, SMS) |
| `fail` | Mark failed | Critical actions (payment) |

```yaml
steps:
  - id: send-email
    action: http
    on_recovery: skip
  - id: create-account
    action: http
    on_recovery: retry
  - id: process-payment
    action: http
    on_recovery: fail
```

### 1.6 Group By (indexed params)

```yaml
params:
  - name: customer_id
    type: string
    required: true
    group_by: true
```

Agent includes `group_by` values in export payload. SaaS indexes in `execution_group_params`. SaaS UI shows grouped views.

### 1.7 SaaS API (new endpoints)

```
# Agent-facing (API key auth)
GET  /api/v1/agent/recovery?agent_id={id}
POST /api/v1/agent/executions/claim

# Dashboard-facing (JWT auth)
GET  /api/v1/groups?{param_name}={value}
GET  /api/v1/groups/{param_name}/{value}/executions
```

### 1.8 SaaS UI — Groups (specific to SaaS frontend)

- **List view**: groups by param name, execution count + status summary
- **Detail view**: all executions for a group value, DAG status per execution
- **Matrix view**: rows = group values, columns = workflows, cells = status icon

### 1.9 TTL Management

Replace ClickHouse native TTL with a scheduled job:

```sql
-- Run daily via cron
DELETE FROM events WHERE event_timestamp < NOW() - INTERVAL retention_days DAY;
DELETE FROM heartbeats WHERE recorded_at < NOW() - INTERVAL 30 DAY;
```

Retention period per organization based on plan (7/30/90 days).

## Changes per repo

### tailflow-shared

| File | Change |
|------|--------|
| `pkg/event/types.go` | Add `ExecutionState` event type |
| `pkg/event/data.go` | Add `ExecutionStateData`, `StepStateData` structs |
| `web/src/types/events.ts` | Add TypeScript equivalents |
| `web/src/types/groups.ts` | New: Group types for SaaS UI |

### tailflow-agent

| File | Change |
|------|--------|
| `internal/parser/schema.go` | Add `GroupBy` to Param, `OnRecovery` to Step, `IdempotencyKey` to HTTPTrigger |
| `internal/parser/parser.go` | Validate `on_recovery` values |
| `internal/runtime/context.go` | Add `Resumed` and `IdempotencyKey` fields |
| `internal/engine/engine.go` | Recovery logic, idempotency resolve, emit execution.state events |
| `internal/export/recovery.go` | New: RecoveryClient |
| `internal/export/claim.go` | New: ClaimClient |
| `internal/server/server.go` | Wire recovery on startup |
| `internal/server/handlers.go` | Wire idempotency claim on HTTP trigger |
| `web/frontend/` | Badges: on_recovery per step, "resumed" on execution |

### tailflow-saas

| File | Change |
|------|--------|
| `internal/database/` | Replace MariaDB + ClickHouse with single TiDB connection |
| `migrations/tidb/001_initial.sql` | All tables consolidated (existing + new) |
| `internal/repository/` | Consolidate to single DB layer, add ExecutionRepository |
| `internal/service/telemetry.go` | Handle `execution.state` in Ingest(), add Recovery(), ClaimExecution() |
| `internal/service/ttl.go` | New: scheduled TTL cleanup job |
| `internal/handler/telemetry.go` | New endpoints: recovery, claim |
| `internal/handler/dashboard.go` | New endpoints: groups |
| `internal/server/router.go` | Register new routes |
| `web/src/views/` | New: GroupsView, GroupDetailView |
| `web/src/components/` | New: GroupsList, GroupMatrix |
| `web/src/stores/` | New: groups store |
| `web/src/router/` | Add groups routes |
| `go.mod` | Remove `clickhouse-go/v2`, keep `go-sql-driver/mysql` |

## Key design decisions

1. **Agent stays stateless** — SaaS is the source of truth
2. **Graceful degradation** — SaaS unreachable = agent works as today
3. **Single database (TiDB)** — replaces MariaDB + ClickHouse, TiFlash for OLAP
4. **group_by is generic** — no domain-specific concepts, just param grouping
5. **Idempotency is opt-in** — only workflows with `idempotency_key`
6. **Recovery is configurable per step** — `on_recovery` controls retry/skip/fail
7. **No agent-side persistence** — agent remains a pure stateless binary
8. **Shared types in tailflow-shared** — single source of truth for agent ↔ SaaS contract
9. **TTL via cron job** — replaces ClickHouse native TTL, per-org retention based on plan

## Future phases

### Phase 2 — Multi-Agent
SaaS becomes DAG scheduler, agents claim steps via pull, fan-out by execution or step.

### Phase 3 — UI & Analytics
Fleet view, timeline view, TiFlash analytics dashboards.
