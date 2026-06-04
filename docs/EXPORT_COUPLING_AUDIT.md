# Export Coupling Audit

Scope: `internal/export` (export ports + noop + store-backed claimer) vs the rest of `tailflow-agent`.

**Status: Path B implemented.** The historical SaaS HTTP code (`exporter.go`, `claim.go`, `recovery.go`, ~680 lines) has been **removed**. What remains in `internal/export` is the dormant seam (interfaces + shared DTOs + noop impls) plus the in-memory idempotency claimer. The idempotency and recovery ports are now backed by local stores; only the event exporter stays noop in v1.

## 1. État actuel — `internal/export`

| Fichier | Rôle |
|---|---|
| `port.go` | Interfaces `EventExporter`, `IdempotencyClaimer`, `ExecutionRecoverer` + DTOs partagés `ClaimResult`, `RecoveredExecution`. Couture dormante : signatures conservées pour un futur SaaS managé. |
| `noop.go` | Impls no-op : `NewNoopExporter`, `NewNoopClaimer`, `NewNoopRecoverer`. |
| `memory_claimer.go` | `MemoryClaimer` : déduplication store-backed via `map[workflow+key]` (mono-instance, fail-closed sur la clé). |

Implémentations store-backed des ports vivant **hors** `internal/export` (pas de cycle : `export` n'importe ni `store` ni `mariadb`) :

| Port | Impl mémoire | Impl MariaDB |
|---|---|---|
| `IdempotencyClaimer` | `export.MemoryClaimer` (`internal/export/memory_claimer.go`) | `(*mariadb.Store).ClaimExecution` (`internal/store/mariadb/idempotency.go`, `UNIQUE uk_*_exec_idem`) |
| `ExecutionRecoverer` | `store.MemoryRecoverer` (`internal/store/memory_recoverer.go`) | `(*mariadb.Store).RecoverExecutions` (`internal/store/mariadb/recoverer.go`, scan lock-free `status IN (running, waiting)`) |
| `EventExporter` | `export.NewNoopExporter()` (v1) | `export.NewNoopExporter()` (v1) |

Le câblage par backend se fait dans `internal/fx/export_ports.go` (`selectClaimer` / `selectRecoverer`) : nil/memory → impls mémoire ; mariadb → `*mariadb.Store` ; défaut → noop. L'exporter est toujours noop.

## 2. Call-sites actuels — hors `internal/export/`

| Fichier:ligne | Symbole | Catégorie |
|---|---|---|
| `internal/server/handlers_triggers.go:85` | `s.config.Claimer.ClaimExecution(ctx, executionID, wf.Name, resolvedKey)` | Claimer (idempotency check) |
| `internal/server/server_lifecycle.go:16` | `s.config.Exporter.Start(ctx)` | Exporter lifecycle (noop) |
| `internal/server/server_lifecycle.go:186` | `s.config.Exporter.Shutdown()` | Exporter lifecycle (noop) |
| `internal/server/server_lifecycle.go:26` | `s.config.Recoverer.RecoverExecutions(ctx, "")` | Recoverer (boot scan) |
| `internal/server/server.go:42,44` | `Exporter export.EventExporter`, `Recoverer export.ExecutionRecoverer` (champs `Config`) | injection via DI, fallback noop si nil |
| `internal/fx/export_ports.go` | `selectClaimer` / `selectRecoverer` + `NewNoopExporter` | construction des ports par backend |
| `internal/fx/server.go` | import `internal/export` (carry des ports dans `server.Config`) | périphérie |

Les ports sont désormais **injectés** (DI via `Config` + provider fx), plus instanciés directement dans le cœur. Plus aucun `export.New`, `export.NewClaimClient`, `export.NewRecoveryClient` : ces constructeurs HTTP SaaS ont été supprimés.

## 3. Verdict sur la frontière

**PROPRE (Path B appliqué).**

- Surface = 3 interfaces + 2 DTOs partagés (`ClaimResult`, `RecoveredExecution`).
- Inversion de dépendance en place : le serveur dépend des interfaces `export.*`, jamais d'une impl concrète. Le choix d'impl est centralisé dans le provider fx.
- Le binaire self-hosted n'embarque plus de code SaaS HTTP : couture dormante (interfaces) seulement.
- Couplage transitif résiduel assumé : `RecoveredExecution.Steps` reste typé `map[string]*runtime.StepResult` (DTO partagé). Acceptable pour v1.

## 4. Reste à faire (post-v1, tracé)

- Implémentation réelle de `EventExporter` (sink distant) quand le SaaS managé reprend : se branche derrière l'interface existante sans toucher engine/server.
- Gating compile-time éventuel via build tag `saas` pour l'impl exporter distante.
- `RecoveredExecution.RawParams` / `RawSteps` / `Status` : champs DTO conservés pour le futur recoverer distant (parsing dual-format).

**Historique** : l'audit d'origine recommandait Path B (`STRIP_IMPL_KEEP_INTERFACE`). C'est désormais fait — code SaaS réel retiré, interfaces conservées comme couture dormante, ports idempotency/recovery implémentés store-backed en M1, seul `EventExporter` reste noop en v1.
