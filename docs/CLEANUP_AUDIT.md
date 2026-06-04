# tailflow-agent — cleanup audit (pre-stable)

Branch: `chore/cleanup-pre-stable` · forked from `feat/execution-timeline` (uncommitted modifs).
Latest commit: `51a0f85 save`. Decision (D3, BRANCHE A — actée): la viz d'exécution officielle est `WorkflowDAGCustom` (custom SVG pan/zoom). `StepTimeline` n'a jamais été câblé et n'existe plus dans l'arbre ; `WorkflowGraph` et `StepNode` ont été supprimés. Aucun recâblage prévu.

---

## 1. Backend (Go) — `internal/*`

### Packages

| Package | Rôle | Status | Notes |
|---|---|---|---|
| `internal/action` | Registry + factories pour toutes les actions du DSL | OK stable | 1 fichier par action, build tag `saas` exclut exec/js/file.* (`builtins_unsafe.go`). `table.go` modifié (uncommitted). |
| `internal/engine` | Executor : DAG, expression eval, goto, sensitive masking, OTel spans | OK stable | `engine.go` modifié (uncommitted). 14 fichiers, bonne couverture test. |
| `internal/event` | Bus pub/sub events (`step.*`, `workflow.*`) | OK stable | `bus.go` a `Subscribe` (lossy) + `SubscribeBlocking` + `Dropped()` counter (cf. STATUS.md). Modifs uncommitted. |
| `internal/parser` | YAML -> `Workflow` struct + `Validate` | WIP a finir | `parser.go` + `schema.go` modifies (uncommitted) — verifier que le validate couvre les nouveaux champs. |
| `internal/runtime` | KV (memory/redis), expression evaluator, locker, dbpool, txregistry, services interfaces | OK stable | Bien decoupe. Pas de modifs en cours. |
| `internal/store` | Persistance in-memory des Executions (`map[id]*Execution`) | WIP a finir | `execution.go` modifie (uncommitted). Pas de persistance disque — par design pour l'agent, mais a acter. |
| `internal/server` | HTTP API + SSE + UI embed + cron + RabbitMQ wait + webhook wait | WIP a finir | `handlers.go`, `routes.go`, `server.go`, `sse.go` tous modifies (uncommitted). 1746 lignes dans `handlers.go` — trop. |
| `internal/trigger` | Resolution des routes HTTP/webhook depuis `wf.Trigger` | OK stable | Petit, cible. |
| `internal/lock` | Lock interface + memory impl | OK stable | Mince, OK. |
| `internal/export` | Exporter SaaS (push events vers backend distant) + claim + recovery | ? incertain | Code present, garde pour la roadmap SaaS — verifier qu'il n'est pas reference par defaut au cas ou l'agent est offline. |
| `internal/metrics` | Collector metriques internes | OK stable | |
| `internal/otel` | Tracing, logging, business metrics, propagation | OK stable | Recemment integre (commit `fd86cbc`). |
| `internal/fx` | Modules uber-fx (DI) | ? incertain | Present mais non cable visiblement dans `cmd/tailflow/main.go` (qui assemble manuellement). A soit utiliser, soit virer. |
| `internal/fake` | Fakes pour tests (action/lock/runtime/store) | OK stable | Test-only. |

### Actions disponibles (`internal/action/*.go`, hors `_test.go`)

| Groupe | Actions | Status |
|---|---|---|
| Control flow | `condition`, `loop`, `delay`, `group` | OK stable |
| State / scope | `set`, `template` | OK stable |
| Data — array | `array.sort`, `array.filter`, `array.map`, `array.uniq`, `array.pick`, `array.concat` | OK stable |
| Data — object/string | `object`, `string.replace`, `string.match_all`, `math`, `hash`, `table` | OK stable |
| JSON | `json.decode`, `json.encode` | OK stable |
| Validation | `validate` | OK stable |
| Logging / response | `log`, `response` | OK stable |
| HTTP | `http` | OK stable |
| Wait (triggers) | `wait.webhook`, `wait.rabbitmq` | OK stable |
| Messaging | `rabbitmq.shovel` | OK stable |
| KV | `kv.get`, `kv.set`, `kv.delete` | OK stable |
| Lock | `lock`, `unlock` | OK stable |
| SQL | `sql.query`, `sql.exec`, `sql.begin`, `sql.commit`, `sql.rollback` | OK stable |
| Schedule | `schedule` | ? incertain (utilite par rapport au trigger schedule) |
| Unsafe (build tag `!saas`) | `exec`, `js`, `file.read`, `file.write` | OK stable |

`registry.go` (82 l) + `builtins.go` (60 l) + `builtins_unsafe.go` (10 l) : architecture propre.

---

## 2. Frontend (Vue 3)

### Vues — `web/frontend/src/views/*.vue`

| Vue | Role | Routee | Status |
|---|---|---|---|
| `DashboardView.vue` (431 l) | Home : workflow header, agent metrics, pipeline DAG mini, recent runs | `/` | OK stable — utilise `WorkflowDAGCustom` (viz officielle, D3 BRANCHE A). |
| `ExecutionView.vue` (403 l) | Vue d'une execution : ActiveRunsStrip + DAG + LogsDock | `/executions/:id` | OK stable — utilise `WorkflowDAGCustom` (viz officielle, D3 BRANCHE A). |
| `ExecutionsView.vue` (312 l) | Historique : filtres, table, compare | `/executions` | OK stable |
| `EditorView.vue` (811 l) | Palette + DAG editable + form step + YAML CodeMirror | `/editor` | OK stable — utilise `WorkflowDAGCustom` en mode edition (viz officielle, D3 BRANCHE A). |
| `DocsView.vue` (669 l) | Docs embarquees avec TOC | `/docs` | OK stable |
| `StepDetailView.vue` | Vue detail d'un step | `/steps/:id` | SUPPRIME (DONE) — `StepInspector` joue ce role dans un slide-in. |
| `WorkflowView.vue` | Vue workflow (graphe + liste) | `/workflow` (redirect) | SUPPRIME (DONE) — la route redirect et la vue ont ete retirees. |

### Composants — `web/frontend/src/components/*.vue`

| Composant | Importe par | Status |
|---|---|---|
| `AppSidebar.vue` (148 l) | `App.vue` | OK stable |
| `AppTopBar.vue` (87 l) | `App.vue` | OK stable |
| `CommandPalette.vue` (182 l) | `App.vue` | OK stable |
| `SettingsPanel.vue` (101 l) | `App.vue` | OK stable |
| `ShortcutsHint.vue` (77 l) | `App.vue` | OK stable |
| `StepInspector.vue` (455 l) | `App.vue` (singleton) | OK stable |
| `LogsDock.vue` (128 l) | `ExecutionView` | OK stable |
| `ExprField.vue` (130 l) | `EditorView` | OK stable |
| `YamlEditor.vue` (89 l) | `EditorView` | OK stable |
| `ParamForm.vue` (97 l) | `App.vue` (run modal) | OK stable |
| `ActiveRunsStrip.vue` (157 l) | `ExecutionView` | OK stable |
| `WorkflowDAGCustom.vue` | `Dashboard`, `Execution`, `Editor` | OK stable — viz d'execution officielle (D3 BRANCHE A). |
| `StepTimeline.vue` | — | SUPPRIME (DONE) — jamais câblé, retiré de l'arbre (D3 BRANCHE A retient `WorkflowDAGCustom`). |
| `WorkflowGraph.vue` | — | SUPPRIME (DONE). |
| `StepNode.vue` | — | SUPPRIME (DONE). |
| `JsonView.vue` | — | SUPPRIME (DONE) — doublon de `@tailflow/shared`. |
| `ExecutionList.vue` | — | SUPPRIME (DONE). |
| `primitives/Icon.vue` | partout | OK stable |
| `primitives/Kbd.vue` | partout | OK stable |
| `primitives/StatusBadge.vue` | Dashboard, Execution(s) | OK stable |

### Composables — `web/frontend/src/composables/*.ts`

| Composable | Importe par | Status |
|---|---|---|
| `useWorkflowApi.ts` (230 l) | toutes les vues + Sidebar/CommandPalette/StepInspector | OK stable |
| `useSSE.ts` (328 l) | `ExecutionView` | OK stable |
| `useGlobalEvents.ts` (196 l) | Sidebar, Dashboard, ExecutionsView, ActiveRunsStrip | OK stable |
| `useStepInspector.ts` (22 l) | App, Dashboard, Execution, Editor | OK stable |
| `useFormat.ts` (37 l) | Sidebar, Dashboard, ExecutionView, ExecutionsView, CommandPalette, StepInspector | OK stable |
| `useSettings.ts` (43 l) | Sidebar, SettingsPanel, WorkflowDAGCustom | OK stable (DAG conserve, D3 BRANCHE A). |
| `useShortcuts.ts` (59 l) | App | OK stable |
| `useTheme.ts` (29 l) | App | OK stable |
| `useRunTrigger.ts` (10 l) | App, Dashboard, WorkflowView, CommandPalette | WIP tres mince ; verifier qu'il a une vraie raison d'exister vs inline. |

### Stores Pinia — `web/frontend/src/stores/`

KILL **vide.** `createPinia()` est appele dans `main.ts` mais aucun store n'est defini. Soit virer Pinia, soit cabler les state partages actuellement eparpilles en composables (ex. `useGlobalEvents` est de facto un store).

### i18n — `web/frontend/src/i18n/`

- `en.ts` et `fr.ts` ont chacun 109 lignes et la meme structure (top-keys identiques : `nav`, `dashboard`, `workflow`, `steps`, `stepDetail`...).
- ~68 usages de `$t(`/`t(` dans les vues et composants ; ~20 `useI18n` calls.
- Couverture : OK structure parallele. Tags `nav.workflow`, `workflow.graph/list` deviennent obsoletes si `WorkflowView` saute.

---

## 3. Endpoints API — `internal/server/routes.go`

| Methode | Path | Handler | Description | Editor-gated |
|---|---|---|---|---|
| GET | `/api/version` | `handleGetVersion` | Version + flag `editor_enabled` | — |
| GET | `/api/metrics` | `handleGetMetrics` | Metriques internes (CPU, goroutines, etc.) | — |
| GET | `/api/workflow` | `handleGetWorkflow` | Workflow info (name, params, trigger...) | — |
| GET | `/api/workflow/raw` | `handleGetWorkflowRaw` | YAML brut | — |
| PUT | `/api/workflow/raw` | `handlePutWorkflowRaw` | Save YAML brut | YES `--editor` |
| GET | `/api/workflow/graph` | `handleGetWorkflowGraph` | Nodes/edges/stages derives du DAG | — |
| GET | `/api/workflow/activity` | `handleGetWorkflowActivity` | Activite agregee (counts par status) | — |
| POST | `/api/workflow/validate` | `handleValidateWorkflow` | Validation d'un YAML candidat | — |
| POST | `/api/workflow/run` | `handleRunWorkflow` | Lance une nouvelle execution | — |
| GET | `/api/workflow/steps/metrics` | `handleGetAllStepMetrics` | Metrics agregees par step | — |
| GET | `/api/workflow/steps/{id}` | `handleGetStepDetail` | Detail d'un step (config + history + metrics) | — |
| GET | `/api/executions` | `handleListExecutions` | Liste paginee des executions | — |
| GET | `/api/executions/{id}` | `handleGetExecution` | Une execution + ses steps | — |
| POST | `/api/executions/{id}/cancel` | `handleCancelExecution` | Cancel une execution en cours | — |
| GET | `/api/executions/{id}/events/list` | `handleListEvents` | Pagination JSON des events d'une exec | — |
| GET | `/api/executions/{id}/events` | `handleSSE` | Stream SSE d'une exec (replay + live) | — |
| GET | `/api/events` | `handleGlobalSSE` | Stream SSE global (toutes execs) | — |
| ANY | `/api/public/...` | `handlePublicTrigger` | Triggers HTTP publics (resolus depuis `trigger`) | — |
| ANY | `/api/wait/...` | `handleWaitWebhook` | Resume `wait.webhook` paused steps | — |
| GET | `/*` | UI (FS embed `web/dist`) | Sert le SPA Vue | — |

`handleIdempotencyCheck` (handlers.go:546) est helper interne, pas route directement.

---

## 4. Recommandations cleanup (top 10, par impact)

1. **DONE — Viz d'execution actee** : `WorkflowDAGCustom` est la viz officielle (D3 BRANCHE A). `StepTimeline` n'a jamais ete câblé et a ete retire ; aucun recâblage prevu.
2. **Commit ou stash le WIP existant** avant cleanup : 30 fichiers modifies non commites sur `chore/cleanup-pre-stable`, on travaille sur du sable.
3. **DONE — `WorkflowGraph.vue` et `StepNode.vue` supprimes** (composants vue-flow morts). `WorkflowDAGCustom.vue` est conserve comme viz officielle (D3 BRANCHE A).
4. **DONE — Composants/vues morts supprimes** : `components/JsonView.vue` (doublon de `@tailflow/shared`), `components/ExecutionList.vue`, `views/StepDetailView.vue` (+ route `/steps/:id`), `views/WorkflowView.vue` (+ route `/workflow`).
5. **Trancher Pinia** : soit creer les stores reels (`useGlobalEvents`, `useSSE`, settings persistes), soit retirer `createPinia` de `main.ts` et la dependance.
6. **DONE — `internal/server/handlers.go` decoupe** : eclate en `handlers_{capture,executions,graph,response,steps,tree,triggers,wait,wf}.go` — handlers <500 l, tests facilites.
7. **Trancher `internal/fx`** : module uber-fx present mais le `cmd/tailflow/main.go` assemble manuellement -> soit on migre vers fx, soit on supprime le module pour eviter la double source de verite.
8. **Trancher `internal/export`** : path SaaS, verifier qu'il n'est jamais active en self-hosted par defaut, sinon retirer du build agent stable et le remettre derriere un build tag.
9. **Nettoyer i18n des cles mortes** (`nav.workflow`, `workflow.graph`, `workflow.list`, `steps.*` cote liste) apres suppression de `WorkflowView` et `StepDetailView`.
10. **Sortir du repo `web/frontend/MEMORY.md` et `web/frontend/docs/`** (notes de session, pas de la doc produit) — deplacer dans `docs/notes/` si a conserver, ou ajouter au `.gitignore`.

---

## Annexes

- Fichiers WIP non commites notables : `internal/engine/engine.go`, `internal/parser/parser.go`, `internal/parser/schema.go`, `internal/server/{handlers,routes,server,sse}.go`, `internal/store/execution.go`, `internal/event/bus.go`, `internal/action/table.go`, `pkg/workflow/types.go`, `cmd/tailflow/main.go`, plus la quasi-totalite du frontend.
- Les nouveaux composants frontend (`ActiveRunsStrip`, `AppSidebar`, `AppTopBar`, `CommandPalette`, `ExprField`, `LogsDock`, `SettingsPanel`, `ShortcutsHint`, `StepInspector`, `WorkflowDAGCustom`, `YamlEditor`, `primitives/*`) et composables (`useFormat`, `useSettings`, `useShortcuts`, `useStepInspector`) sont **untracked** — ils doivent passer dans le commit de cleanup.
- `tailwind.config.js`, `package.json`, `package-lock.json`, `index.html` modifies : a inclure dans le commit cleanup pour que le build reste stable.
