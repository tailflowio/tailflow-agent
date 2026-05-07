# Export Coupling Audit

Scope: `internal/export/{claim,exporter,recovery}.go` (~680 lines of prod code) vs the rest of `tailflow-agent` on branch `chore/cleanup-pre-stable`.

## 1. Surface invoquée — call-sites hors `internal/export/`

| Fichier:ligne | Symbole | Catégorie | Criticité |
|---|---|---|---|
| `cmd/tailflow/main.go:24` | `import .../internal/export` | import | périphérie |
| `cmd/tailflow/main.go:1281` | `*export.Exporter` (field `runResources.exporter`) | field decl | périphérie |
| `cmd/tailflow/main.go:1298` | `rr.exporter.Shutdown()` | fn call (lifecycle) | périphérie |
| `cmd/tailflow/main.go:1439` | `(*export.Exporter, …)` (return type `setupExporter`) | construction | périphérie |
| `cmd/tailflow/main.go:1447` | `export.New(export.Config{…})` | construction | périphérie |
| `cmd/tailflow/main.go:1463` | `exporter.Start(exportCtx)` | lifecycle | périphérie |
| `cmd/tailflow/main.go:1448-1460` | `export.Config{ExportURL,APIKey,AgentName,EventBus,Logger,WorkflowName,WorkflowDescription,WorkflowTags,TriggerType,StepsCount,Version,Revision}` | config carry | périphérie |
| `internal/server/server.go:16` | `import .../internal/export` | import | cœur |
| `internal/server/server.go:62` | `*export.Exporter` (field `Server.exporter`) | field decl | cœur |
| `internal/server/server.go:168` | `export.New(export.Config{…})` | construction | cœur |
| `internal/server/server.go:182` | `s.exporter.Start(ctx)` | lifecycle | cœur |
| `internal/server/server.go:192` | `export.NewRecoveryClient(...)` + `RecoverExecutions(...)` | construction + fn call | cœur |
| `internal/server/server.go:202-229` | `RecoveredExecution` field reads (`WorkflowName`, `ExecutionID`, `Steps`, `Params`) | type read | cœur |
| `internal/server/server.go:341-342` | `s.exporter.Shutdown()` | lifecycle | cœur |
| `internal/server/handlers.go:19` | `import .../internal/export` | import | cœur |
| `internal/server/handlers.go:569` | `export.NewClaimClient(...)` | construction | cœur |
| `internal/server/handlers.go:571` | `claimClient.ClaimExecution(...)` | fn call | cœur |
| `internal/server/handlers.go:577-583` | `ClaimResult.Claimed`, `.ExistingExecutionID`, `.ExistingStatus` | type read | cœur |
| `cmd/tailflow/main.go:1868-1870` | `Config{ExportURL, APIKey, ExporterName}` carried into `server.Config` | config carry | cœur |

Pas d'autres call-sites: `grep "internal/export"` ne renvoie que les 3 imports ci-dessus.

## 2. Surface réelle vs surface exposée

Symboles publics réellement consommés depuis l'extérieur:

- **Types**: `Exporter`, `Config` (12 champs lus), `RecoveryClient`, `RecoveredExecution` (4 champs lus: `ExecutionID`, `WorkflowName`, `Steps`, `Params`), `ClaimClient`, `ClaimResult` (3 champs lus).
- **Constructeurs**: `New(Config)`, `NewRecoveryClient(baseURL, apiKey)`, `NewClaimClient(baseURL, apiKey)`.
- **Méthodes**: `(*Exporter).Start(ctx)`, `(*Exporter).Shutdown()`, `(*RecoveryClient).RecoverExecutions(ctx, agentID)`, `(*ClaimClient).ClaimExecution(ctx, executionID, workflowName, idempotencyKey)`.

Surface exportée non utilisée (candidats à privatisation): `RecoveredExecution.RawParams`, `RecoveredExecution.RawSteps`, `Status` (sur `RecoveredExecution`). Tout le reste du fichier `exporter.go` (registration/heartbeat/batch loop/`applyServerConfig`/`post`) est **déjà non-exporté**, donc la surface publique est étonnamment fine: 3 constructeurs + 4 méthodes + 3 types-DTO. Le couplage est donc "petit en surface", "gros en sémantique" (le `Config` porte 12 champs métier).

## 3. Verdict sur la frontière

**MIXTE.**

Justification: la surface publique de `internal/export` est minimaliste (3 constructeurs + 4 méthodes), ce qui plaide pour une frontière propre. MAIS (a) `internal/server` importe et instancie directement `export.New`, `export.NewClaimClient`, `export.NewRecoveryClient` à 3 endroits du cœur (server.go + handlers.go) — pas via DI; (b) `RecoveredExecution.Steps` expose `runtime.StepResult` ce qui crée un couplage transitif `export → runtime`; (c) le gating actuel est uniquement runtime (`if ExportURL == ""`), donc le binaire stable embarque le code SaaS même quand il n'est jamais exécuté. Frontière correcte côté API mais inversion de dépendance manquante côté server.

## 4. Qualité du code (sanity check)

**`exporter.go` (454 l)** — async, 3 goroutines (`register`/`batchLoop`/`heartbeatLoop`), buffer cap 10k events, chunking 2k/req, exponential backoff sur registration (1s→30s), config push-back depuis le SaaS via `applyServerConfig`. Buffer `[]event.Event` non protégé par mutex (lu/écrit uniquement dans `batchLoop`, OK). Pas de circuit breaker. Couplage interne: dépend de `internal/event.Bus`. Red flags mineurs: `finalFlush` peut perdre des events après 5s d'attente registration; `Shutdown()` ne ferme pas le ctx lui-même (responsabilité de l'appelant via `exportCancel`), donc Shutdown sans cancel = leak — c'est exactement ce que fait `runResources.shutdown()` (cancel puis Shutdown), bonne pratique côté caller mais fragile.

**`claim.go` (73 l)** — sync HTTP, fail-open (toute erreur → `Claimed: true` = laisse passer l'exec). Couplage interne: zéro. Red flag: l'`err` de `json.Marshal` est ignoré silencieusement (`return &ClaimResult{Claimed: true}, nil`) — le caller ne peut pas distinguer "SaaS unreachable" de "marshal failed". Comportement assumé pour fail-open mais à documenter.

**`recovery.go` (153 l)** — sync HTTP GET + parsing dual-format (array de `rawStep` OU map). Couplage interne: dépend de `internal/runtime.StepResult` et `runtime.StepError` — fuite de types runtime dans la signature publique de `RecoveredExecution`. Pas de retry. Red flag: aucun. Bien isolé.

## 5. Recommandation

**Path B — `STRIP_IMPL_KEEP_INTERFACE`** (avec un soupçon de Path C sur `RecoveredExecution`).

Justification: la surface est déjà étroite (§2), le gating runtime est déjà en place, l'effort pour C n'est pas justifié pour une v stable self-hosted. B donne (i) un binaire self-hosted qui n'embarque pas le code SaaS, (ii) une cible d'archi propre quand le SaaS reprend, (iii) testabilité accrue (mock `EventExporter`). Path A est le minimum mais laisse ~680 lignes de code SaaS dans le binaire stable. Path C est sur-ingénierie pour le périmètre actuel (3 call-sites cœur).

### Sketch Path B

- Nouveaux fichiers:
  - `internal/export/port.go` — interface `EventExporter { Start(ctx); Shutdown() }`, interface `IdempotencyClaimer { ClaimExecution(...) (*ClaimResult, error) }`, interface `ExecutionRecoverer { RecoverExecutions(...) ([]RecoveredExecution, error) }`. Les types `ClaimResult` et `RecoveredExecution` restent ici (DTO partagés).
  - `internal/export/noop.go` — impls no-op (`noopExporter`, `noopClaimer`, `noopRecoverer`) + `NewNoop*()` constructeurs. ~60 lignes.
  - `internal/export/saas/` (sous-package) — déplacer `exporter.go`, `claim.go`, `recovery.go` actuels. Renommer `New` → `NewExporter`, etc. ~680 lignes déplacées, ~10 lignes diffées (imports + extraction `RecoveredExecution`/`ClaimResult` qui remontent au parent).
- Déplacements:
  - `RecoveredExecution.Steps` reste typé `map[string]*runtime.StepResult` (couplage assumé sur le DTO partagé) OU passe en `map[string]any` parsed côté server (réduit le couplage transitif — recommandé).
- Modifs `server.Config`: ajout de 3 champs `Exporter EventExporter`, `Claimer IdempotencyClaimer`, `Recoverer ExecutionRecoverer`. `New()` nil-check → fallback noop.
- Modifs `cmd/tailflow/main.go`: choix entre `saas.NewExporter(...)` ou `export.NewNoopExporter()` selon `ExportURL`. Gating compile-time possible via build tag `saas` plus tard.
- Tests `_test.go` actuels restent dans `internal/export/saas/` (déplacement uniquement).
- **Fichiers touchés**: ~8 (3 dans export à splitter, 1 server.go, 1 handlers.go, 1 main.go, 2 nouveaux). **Lignes diffées**: ~150 (hors déplacements). **Effort**: ~3-4h. **Risque de régression**: faible — refactor mécanique, tests existants couvrent la logique métier; seul risque réel = oublier un `nil` check sur les nouvelles interfaces côté server lifecycle. Audit suggéré: vérifier que `s.exporter.Shutdown()` reste appelé même quand l'impl est noop (trivial via interface).
