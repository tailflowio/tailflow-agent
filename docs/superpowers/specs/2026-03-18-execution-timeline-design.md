# Execution Timeline View - Design Spec

## Goal

Replace the DAG graph + step results list in ExecutionView with a single vertical timeline that mirrors the CLI rendering experience. Unified visual language between CLI and web.

## Architecture

### What Gets Removed from ExecutionView
- `WorkflowGraph` import and usage in ExecutionView
- DAG collapsible section in ExecutionView template
- Step results collapsible section in ExecutionView template
- Related refs: `dagAutoHeight`, `dagCollapsed`, `onGraphReady`, `onDagStepClick`, `resultsCollapsed`, `toggleResults`, `focusedStepId`

Note: `WorkflowGraph.vue` and `StepNode.vue` are NOT deleted — they are still used by `WorkflowView.vue`. Vue Flow and dagre dependencies stay in package.json.

### What Gets Added
- `StepTimeline.vue` — new component, replaces both DAG and results in ExecutionView

### What Stays
- `ExecutionView.vue` — header, stats row, cancel banner, SSE connection, event timeline
- `EventTimeline.vue` (shared) — stays at the bottom
- `JsonView.vue` (shared) — used inside expanded steps
- All SSE composables (`useSSE`) — data source unchanged
- `api.getWorkflowGraph()` call — kept for step ordering (provides DAG node order)

## Step Ordering

The `graph` ref and `api.getWorkflowGraph()` call are kept in ExecutionView solely to derive step order. The computed `orderedSteps` continues to use `graph.value.nodes` to list all steps in DAG order. This ensures steps appear in the correct workflow-defined order even before they start executing.

## StepTimeline Component

### Props
```typescript
interface Props {
  steps: [string, StepResult][]
  stepStatuses: Record<string, string>
  stepVolumes: Record<string, StepVolume>
  stepIterations: Record<string, number>
  stepOutputHistory: Record<string, StepOutputEntry[]>
  stepPipelineProgress: Record<string, PipelineEntry[]>
  events: WorkflowEvent[]
}
```

### Visual Structure

Each step is a row in a single container (`bg-g-2 border border-g-5 rounded-lg`).

**Compact row (default):**
```
[dot 6px] [step_name mono] .................. [action chip] [duration]
```
- Dot color by status:
  - success: `#34d399` (emerald)
  - failed: `#f87171` (red)
  - running/waiting: `#fbbf24` (amber)
  - pending: `#666` (gray)
  - skipped: `#6b7280` (gray)
  - cancelled: `#fb923c` (orange)
- Running dot has glow (`box-shadow: 0 0 8px`)
- Running row has left border accent (`border-left: 2px solid amber`) and subtle bg highlight
- Failed row has red left border and red-tinted bg
- Cursor pointer — clickable to expand

**Expanded row (on click):**
Same header row with chevron indicator + expanded content below, indented (`padding-left: 32px`):
- **Input** section: `JsonView` component, only if input data exists
- **Output** section: `JsonView` component, only if output data exists
  - Multi-iteration: collapsible per-iteration with iteration badge and timestamp
- **Error** section: red border box with error message, only if error exists
- **Logs** section: filtered `step.log` events for this step ID, monospace block, only if logs exist

Sections only render when they have content — no empty headers.

**Loop separator:**
Inserted in the timeline between iterations when `stepIterations[stepId] > 1` and a `step.goto` event triggers a new cycle. Appears as a row in the compact timeline:
```
  ↻ iteration 2/10
```
Left border accent in amber. Not clickable. Visually separates repeated step cycles.

**Pipeline progress (for loop actions):**
Shown inline under a running/completed step that has pipeline data. Same rendering as current view: iteration groups with action dots (green ok, red failed, gray running).

### Behavior

- Steps listed in DAG order (from `orderedSteps`), all visible from the start
- Pending steps shown grayed out with gray dot
- As SSE events arrive, steps transition: pending → running → success/failed
- Clicking a step toggles expand/collapse (accordion: only one expanded at a time)
- Deferred rendering for expanded content (double rAF pattern, already used in current code)
- Running step gets visual emphasis (amber left border + bg highlight) but does not auto-expand

## ExecutionView Changes

### Remove
- `WorkflowGraph` import and template usage
- `dagAutoHeight`, `dagCollapsed`, `onGraphReady`, `onDagStepClick` refs/functions
- DAG collapsible card in template
- `resultsCollapsed`, `toggleResults` refs/functions
- Step results collapsible card in template
- `focusedStepId` ref and scroll logic
- `expandedSteps`, `deferredSteps`, `expandedIterations` refs (moved to StepTimeline)
- `isStepExpanded`, `isStepContentReady`, `deferContent`, `toggleStep`, `collapseAll`, `toggleIteration`, `isIterationExpanded` functions (moved to StepTimeline)

### Keep
- `graph` ref and `api.getWorkflowGraph()` call (for step ordering)
- `orderedSteps` computed (passed to StepTimeline)
- Header (workflow name, execution ID, duration, live badge, status badge)
- Cancel banner (recovery)
- Stats row (duration, steps, started)
- Event timeline at bottom
- All SSE logic (`useSSE` composable)
- `cancelExec` functionality

### Add
- `StepTimeline` import and usage between stats row and event timeline
- Pass `orderedSteps`, SSE reactive data, and `eventRows` as props

## Files Changed

| File | Action |
|------|--------|
| `web/frontend/src/views/ExecutionView.vue` | Rewrite — remove DAG/results sections, add StepTimeline |
| `web/frontend/src/components/StepTimeline.vue` | New component |

## What This Does NOT Change
- `WorkflowGraph.vue` — untouched (used by WorkflowView)
- `StepNode.vue` — untouched (used by WorkflowGraph)
- `ExecutionsView` (list page) — untouched
- `EventTimeline` — untouched
- SSE composable — untouched
- API endpoints — untouched
- Backend — untouched
- `package.json` — untouched (vue-flow/dagre still needed by WorkflowView)
