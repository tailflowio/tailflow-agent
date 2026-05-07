# Stages Feature - Design Spec

## Goal

Add mandatory `stages` to workflow YAML for visual grouping. Steps reference a stage by name. UI renders stages as horizontal GitLab-style pipeline columns. `depends_on` remains the execution model — stages are purely organizational.

## YAML Schema

```yaml
version: "2.0"
name: "pull-all"

stages:
  - name: init
    description: "Initialize environment"
  - name: fetch
    description: "Fetch data from GitLab API"
  - name: process
    description: "Clone and sync projects"
  - name: report
    description: "Generate summary"

steps:
  - id: tag-sync
    stage: init
    action: group
  - id: init
    stage: init
    action: set
  - id: fetch_page
    stage: fetch
    depends_on: [init, tag-sync]
    action: set
  - id: summary
    stage: report
    depends_on: [sync_projects]
    action: table
```

## Data Model

### New types in parser/schema.go

```go
type Stage struct {
    Name        string `json:"name"                  yaml:"name"`
    Description string `json:"description,omitempty"  yaml:"description,omitempty"`
}
```

### Changes to existing types

- `Workflow`: add `Stages []Stage` field
- `Step`: add `Stage string` field

## Validation Rules

- `stages` is required (at least one stage)
- Each stage must have a unique non-empty `name`
- Every step must reference an existing stage via `stage` field
- Stage order in YAML defines display order (left to right)

## Engine Impact

None. The engine uses `depends_on` and the DAG for execution order. `stage` is ignored by the engine.

## Graph API Changes

The `/api/workflow/graph` response adds a `stages` field:

```json
{
  "nodes": [...],
  "edges": [...],
  "tree": [...],
  "stages": [
    {
      "name": "init",
      "description": "Initialize environment",
      "steps": ["tag-sync", "init"]
    },
    {
      "name": "fetch",
      "description": "Fetch data from GitLab API",
      "steps": ["fetch_page", "gitlab_api", "filter_projects", "accumulate", "save_projects", "check_next_page"]
    }
  ]
}
```

Steps within a stage are ordered by their position in the `steps` array (YAML order).

## Frontend

### StepTimeline (ExecutionView)

Horizontal layout — each stage is a column:
- Stage header: name + description + total duration + progress bar
- Loop stages: amber left border + iteration badge + progress dots/bar
- Step cards: SVG status icon + name + label + action chip + duration
- Adaptive iteration display: dots for ≤10, progress bar for >10
- Click step card to expand input/output/error/logs
- Running steps: amber highlight with pulse animation
- Pending steps: dimmed with dashed circle
- Conditional steps: purple `if` badge, skipped steps grayed

### WorkflowView

Same horizontal stage layout but without execution status — all steps show as pending/neutral.

## Files Changed

### tailflow-agent

| File | Change |
|------|--------|
| `internal/parser/schema.go` | Add `Stage` type, add `Stages` to `Workflow`, add `Stage` to `Step` |
| `internal/parser/parser.go` | Add `validateStages` validation |
| `internal/parser/parser_test.go` | Add stage validation tests |
| `internal/server/handlers.go` | Add stages to graph API response |
| `pkg/workflow/types.go` | Add `StageInfo` type to graph response |
| `web/frontend/src/composables/useWorkflowApi.ts` | Add `StageInfo` TS type |
| `web/frontend/src/components/StepTimeline.vue` | Rewrite to horizontal stage layout |
| `web/frontend/src/views/WorkflowView.vue` | Rewrite to horizontal stage layout |
| `web/frontend/src/views/ExecutionView.vue` | Pass stages data to StepTimeline |
| `examples/*.yaml` (28 files) | Add `stages` + `stage` to all workflows |

## Migration

All 28 example workflows must be updated with `stages` and `stage` on every step. Simple workflows get a single "default" stage. Complex workflows get logical stage groupings.
