# Plan d'implémentation — tailflow-agent v1 stable (self-hosted, mono-instance, sans SaaS)

> Plan dérivé du spec `docs/superpowers/specs/2026-06-04-tailflow-v1-stable-design.md`.
> Produit par workflow (7 planificateurs + critique adversarial + synthèse), 2026-06-04.

## 1. Résumé exécutif

Trois jalons séquentiels. **M0** (sécuriser & dégraisser) : purge ClickHouse, décommission du footgun `SelfHosted` → `--unsafe` secure-by-default, et 5 corrections `assign-in-if` + clarification doc `schedule`. **M1** (cœur fonctionnel bloquant v1) : Claimer d'idempotency réel (mémoire + MariaDB, store-backed, contrainte UNIQUE, pas de GET_LOCK) avec fix du bug d'ID d'exécution, et Recoverer réel (scan lock-free au boot) + filets de couverture. **M2** (finitions) : fix goto-reset SSE (UI stale), découpe `engine.go` (1341 l) et `server.go` (504 l) sous 500 l, resync docs UI/SaaS. Le code change réellement en M0 (suppressions + renommage flag + refactor), M1 (nouveaux claimers/recoverers + schéma MariaDB + câblage fx) et M2 (event SSE + relocation pure de symboles + handler FE). Gate global : `go build ./...` + `go test -race ./...` vert à chaque fin de jalon.

## 2. Graphe de dépendances (cross-workstream)

### Fichiers chauds (sérialiser les écritures)
- **`internal/fx/export_ports.go`** : touché par M1a-9 (Claimer) puis M1c-1 (Recoverer). **Séquentiel** : M1a-9 avant M1c-1.
- **`internal/fx/execution_store.go` / `module.go`** : M0 (CH-2, M0b-saas-2/3) puis M1 ne le re-touche pas (seul `export_ports.go` côté fx en M1). OK.
- **`internal/engine/engine.go`** : `assign-in-if` (M0a-2, lignes 1273/1309/1325) **doit précéder** la découpe M2 (M2b-3..6) et le fix SSE M2-SSE-1/M2b-1. Sinon fix-on-move forcé dans M2b-6.
- **`internal/server/handlers_triggers.go`** : M1a-2 (fix ID) puis M1a-9 (Add tolérant duplicate-key). **Séquentiel**.
- **`internal/store/mariadb/schema.go` / `store_test.go`** : M1a-5 (colonne idempotency) puis M1b-2/M1a-2-cov (couverture). Séquentiel léger.
- **`internal/export/port.go` / `noop.go`** : réécrits par M0b-saas-7/8 (M0) ET M1a-0 (M1). **Un seul passage** : faire la réécriture en M0b-saas-7/8 et faire de M1a-0 un simple alignement/no-op si déjà fait (M1a-0 absorbé).

### Ordre recommandé
1. **M0 en parallèle sur 3 lots indépendants** (fichiers disjoints) :
   - Lot ClickHouse (CH-3→CH-2→CH-1→CH-7, et CH-6→CH-5→CH-4 ; CH-8 indép ; CH-9 final).
   - Lot SaaS-footgun (M0b-saas-1→2/3/4/5/6, +7/8 export, M0b-saas-9 final).
   - Lot quick-fixes (M0a-0→M0a-1 ; M0a-2 ; M0a-3 final ; M0b-1 doc indép).
   - **Gate M0** : `go build ./... && go test ./...` + audits grep.
2. **M1 séquentiel sur les fichiers chauds, parallèle ailleurs** :
   - Idempotency (M1a-1→M1a-2 ; M1a-3→M1a-4 ; M1a-5→M1a-6→M1a-7 ; M1a-8→M1a-9 ; M1a-10) ; M1a-11 final.
   - Recovery (M1b-1 ; M1b-2 ; M1c-1 après M1a-9 car même `export_ports.go` ; M1d-1 ; M1e-1).
   - Filets couverture (M1cov-noop, M1cov-mariadb) parallélisables tôt.
   - **Gate M1** : critères fonctionnels (double trigger même clé → 1 exec ; exec running au boot → reprise).
3. **M2 séquentiel sur engine.go** (M2b-1→M2b-3→M2b-4→M2b-5→M2b-6), SSE FE (M2b-2) après M2b-1, server split (M2b-7) parallèle, docs (M2c-*) parallèles. **Gate M2 final**.

### Parallélisable vs séquentiel (synthèse)
| Peut être parallèle | Doit être séquentiel |
|---|---|
| Les 3 lots M0 (fichiers disjoints) | Chaîne ClickHouse CH-2→CH-1 (fenêtre de build cassé) |
| Idempotency vs Recovery vs Couverture en M1 | `export_ports.go` : M1a-9 → M1c-1 |
| Docs M2 (M2c-1/2/3) | `engine.go` : M0a-2 → M2b-* (relocations) |
| M2b-7 (server) vs M2b-* (engine) | `handlers_triggers.go` : M1a-2 → M1a-9 |

---

## 3. Plan par jalon

## JALON M0 — Sécuriser & dégraisser

### Lot A — Quick fixes (assign-in-if + doc schedule)

**M0a-0 — Garde-fou TDD `handlePutWorkflowRaw`**
- Fichiers : `internal/server/handlers_test.go`
- Changement : ajouter dans `HandlersTestSuite` des cas couvrant les 2 branches d'erreur de `handlePutWorkflowRaw` (route `PUT /api/workflow/raw`) : body valide-au-parse mais invalide-à-`Validate()` → 400 `ValidateResponse{Valid:false,Errors:[...]}` ; échec `os.WriteFile` (pointer `s.config.FilePath` sur un dir non-inscriptible 0o500, `EditorEnabled=true`) → 500 ; optionnel `EditorEnabled=false` → 403, body vide → 400. Doit être **vert sur le code actuel** (garde-fou).
- Tests : c'est la tâche de test (TDD avant refactor).
- Verify : `go build ./... && go test ./internal/server/... -run PutWorkflowRaw`
- Dépend de : —

**M0a-1 — Séparer assign-in-if `handlers_wf.go`**
- Fichiers : `internal/server/handlers_wf.go`
- Changement : lignes 85 et 90 de `handlePutWorkflowRaw`. Remplacer `if validateErr := parser.Validate(wf); validateErr != nil {` par `validateErr := parser.Validate(wf)` puis `if validateErr != nil {`. Idem `writeErr := os.WriteFile(s.config.FilePath, body, 0o644)` puis `if writeErr != nil {`. Blocs internes identiques, sémantique constante.
- Tests : garde-fou M0a-0 reste vert avant/après.
- Verify : `go build ./... && go test ./internal/server/...`
- Dépend de : M0a-0

**M0a-2 — Séparer assign-in-if `engine.go` (1273, 1309, 1325)**
- Fichiers : `internal/engine/engine.go`
- Changement : 3 motifs `if err := partialMatch(...); err != nil { return err }` → `err := partialMatch(...)` puis `if err != nil`. Ligne 1273 (`checkTestExpect`), 1309 et 1325 (`partialMatch`, dans boucles `for` — garder `err :=`). Aucun `err` en scope avant chaque site (vérifié).
- Tests : aucun nouveau ; `engine_test.go` couvre toutes les branches `partialMatch`/`checkTestExpect`.
- Verify : `go build ./... && go test ./internal/engine/...`
- Dépend de : —

**M0a-3 — Vérification globale vet/lint**
- Fichiers : `internal/server/handlers_wf.go`, `internal/engine/engine.go`
- Changement : confirmer 0 assign-in-if résiduel : `rg "if [a-zA-Z_]+ :?= .*; .* != nil" internal/server/handlers_wf.go internal/engine/engine.go` doit être VIDE.
- Verify : `go build ./... && go vet ./... && go test ./...`
- Dépend de : M0a-1, M0a-2

**M0b-1 — Clarifier nommage `schedule` (action delay/at vs Trigger cron) — doc/godoc only**
- Fichiers : `internal/action/schedule.go`, `internal/parser/schema.go`, `README.md`, `examples/delay-schedule.yaml`
- Changement : **aucun renommage de mot-clé YAML**. Enrichir godoc `ScheduleTrigger` (schema.go:140) : cron récurrent, distinct de l'action one-shot. Enrichir godoc `ScheduleAction` (schedule.go:9). Ajouter godoc `NewScheduleAction` (schedule.go:12, absent) commençant par l'identifiant. README l.371 (table actions) + l.435 (table triggers) + l.626 (commentaire YAML) : note de distinction. `examples/delay-schedule.yaml` : commentaire inline sur le step one-shot (ne pas ajouter de trigger cron).
- Tests : doc only ; suites parser/action restent vertes.
- Verify : `go build ./... && go test ./internal/parser/... ./internal/action/... && go doc ./internal/parser ScheduleTrigger && go doc ./internal/action ScheduleAction NewScheduleAction`
- Dépend de : —

### Lot B — Suppression ClickHouse

**CH-3 — (TDD) Retirer le cas ClickHouse du test fx execution_store**
- Fichiers : `internal/fx/execution_store_test.go`
- Changement : tests = **méthodes** sur `ExecutionStoreTestSuite` (testify/suite). Supprimer `TestNewExecutionStore_ClickHousePropagatesConnectError` (80-95). Garder `TestNewExecutionStore_UnknownBackendRejected`. Ajouter `TestNewExecutionStore_ClickHouseTypeRejected` utilisant le **littéral `"clickhouse"`** (pas `parser.PersistenceClickHouse`, supprimé en CH-4) : `_, err := NewExecutionStore(ExecutionStoreIn{Config: Config{MaxExecs:100}, Workflow: &parser.Workflow{Persistence: &parser.Persistence{Type:"clickhouse"}}})` sur sa ligne, puis `s.Require().Error(err)` puis `s.Contains(err.Error(), "unknown backend")`.
- Verify : `go test ./internal/fx/...`
- Dépend de : —

**CH-2 — Retirer le wiring ClickHouse du provider fx**
- Fichiers : `internal/fx/execution_store.go`
- Changement : supprimer l'arm `case cfg.Type == parser.PersistenceClickHouse: return newClickHouseStore(in)` (43-44), la fonction `newClickHouseStore`+godoc (70-87), l'import `internal/store/clickhouse` (9). Le default renvoie `persistence: unknown backend %q`. Préserver `newMariaDBStore` (style non-assign-in-if).
- Verify : `go build ./internal/fx/... && go test ./internal/fx/...`
- Dépend de : CH-3

**CH-1 — Supprimer le package store/clickhouse**
- Fichiers : `internal/store/clickhouse/{store,executions,events,schema,store_test}.go`
- Changement : `git rm -r internal/store/clickhouse`. Seul consommateur de `ClickHouse/clickhouse-go/v2`. **Ne pas builder entre CH-2 et CH-1** ; builder une seule fois après CH-1.
- Verify : `test ! -d internal/store/clickhouse && go build ./... && go test ./internal/store/...`
- Dépend de : CH-2

**CH-6 — (TDD) Retirer les tests ClickHouse du parser**
- Fichiers : `internal/parser/parser_test.go`
- Changement : méthodes sur `ParserTestSuite`. Supprimer `TestValidate_PersistenceClickHouse` (851-858), `...MissingSubBlock` (860-866), `...MissingDSN` (868-877). Dans `TestValidate_PersistenceMariaDBWithExtraSubBlockRejected` (887-897) remplacer le sous-bloc `ClickHouse:&ClickHousePersistence{...}` (892) par `Memory:&MemoryPersistence{MaxExecutions:1}` (préserve la garde cross-backend). Garder `TestValidate_PersistenceUnknownType`.
- Verify : `go test ./internal/parser/...`
- Dépend de : —

**CH-5 — Retirer la branche ClickHouse de la validation parser**
- Fichiers : `internal/parser/parser.go`
- Changement : l.80 `if p.MariaDB != nil || p.ClickHouse != nil` → `if p.MariaDB != nil` ; l.99 idem pour `p.Memory`. Supprimer le `case PersistenceClickHouse:` (105-118). l.121 message → `is not supported (memory, mariadb)` (garder substring `not supported`).
- Verify : `go build ./internal/parser/... && go test ./internal/parser/...`
- Dépend de : CH-6

**CH-4 — Retirer ClickHouse du schéma parser**
- Fichiers : `internal/parser/schema.go`
- Changement : supprimer const `PersistenceClickHouse` (28), champ `ClickHouse` de `Persistence` (38), struct `ClickHousePersistence`+godoc (54-59). `gofmt` pour réaligner les tags. Garder Memory/MariaDB.
- Verify : `go build ./internal/parser/... && go test ./internal/parser/...`
- Dépend de : CH-5, CH-6

**CH-7 — Retirer le driver ClickHouse de go.mod/go.sum**
- Fichiers : `go.mod`, `go.sum`
- Changement : `go mod tidy`. Retire `clickhouse-go/v2 v2.46.0` (go.mod:8) et `ch-go v0.71.0` indirect (go.mod:39). **Ne pas éditer go.sum à la main.** Garder `go-sql-driver/mysql`. Revoir le diff (seuls modules ClickHouse retirés).
- Verify : `go mod tidy && ! grep -qi clickhouse go.mod && go build ./...`
- Dépend de : CH-1, CH-2

**CH-8 — Nettoyer les commentaires ClickHouse**
- Fichiers : `internal/server/server.go`, `internal/store/execution.go`
- Changement : comment-only. server.go:209 'SQL/ClickHouse backends'→'SQL backends'. execution.go:33 → 'may be in-memory or SQL (MariaDB/MySQL).' (drop Postgres délibéré). :366 '(SQL, ClickHouse)'→'(SQL)'. :389 '(mariadb, clickhouse)'→'(mariadb)'.
- Verify : `go vet ./internal/server/... ./internal/store/... && go build ./...`
- Dépend de : —

**CH-9 — Gate ClickHouse + audit grep**
- Verify : `go build ./... && go test ./... && ! grep -rIi clickhouse --include='*.go' . && ! grep -qi clickhouse go.mod && ! grep -qi clickhouse go.sum`
- Dépend de : CH-1..CH-8
- Note : laisser les `clickhouse` du spec et des docs SaaS historiques (hors scope).

### Lot C — Décommission footgun SaaS (`SelfHosted` → `--unsafe`)

**M0b-saas-1 — (TDD red) Réécrire `action_registry_test.go` pour `--unsafe`**
- Fichiers : `internal/fx/action_registry_test.go`
- Changement : renommer `...DefaultAppliesSaaSAllowlist`→`...DefaultAppliesSafeAllowlist` (`Config{Unsafe:false}`, `Create("exec")`→error). `...SelfHostedKeepsEverything`→`...UnsafeKeepsEverything` (`Config{Unsafe:true}`, `Create("exec")`→no error). `TestSaaSAllowedActions_...`→`TestSafeAllowedActions_...`, appel `saasAllowedActions`→`safeAllowedActions`. Garder les 6 assertions. Doit échouer à la compilation avant M0b-saas-2/3.
- Verify : `go test ./internal/fx/... -run TestActionRegistryTestSuite` (échoue avant 2/3, passe après)
- Dépend de : —

**M0b-saas-2 — `Config.SelfHosted` → `Config.Unsafe`**
- Fichiers : `internal/fx/module.go`
- Changement : remplacer le champ `SelfHosted bool` (24) par `Unsafe bool` + godoc commençant par `Unsafe`, secure-by-default.
- Verify : `go build ./internal/fx/...`
- Dépend de : M0b-saas-1

**M0b-saas-3 — Inverser l'allowlist sur `Unsafe` + renommer `safeAllowedActions`**
- Fichiers : `internal/fx/action_registry.go`
- Changement : l.24 `if !in.Config.SelfHosted {`→`if !in.Config.Unsafe {` (**garder le `!`** : Unsafe=false → allowlist ACTIVE). l.25 `safeAllowedActions(...)`. Renommer la fonction (33) + godoc sans réf SaaS. Map `blocked` inchangée.
- Verify : `go build ./internal/fx/... && go test ./internal/fx/... -run TestActionRegistryTestSuite`
- Dépend de : M0b-saas-1

**M0b-saas-4 — Flag `--selfhosted` → `--unsafe` dans cmd_serve.go**
- Fichiers : `cmd/tailflow/cmd_serve.go`
- Changement : var `selfHosted`→`unsafe` (19), passage à `executeServe` (30), `cmd.Flags().BoolVar(&unsafe,"unsafe",false,"Disable the default action allowlist: allow exec, js and file.* actions (use only on trusted self-hosted instances)")` (35), param `executeServe` (41), littéral `Config{Unsafe: unsafe}` (49).
- Verify : `go build ./cmd/... && go vet ./cmd/...`
- Dépend de : M0b-saas-2

**M0b-saas-5 — `module_test.go` (SelfHosted → Unsafe)**
- Fichiers : `internal/fx/module_test.go`
- Changement : l.39 `SelfHosted:true`→`Unsafe:true` dans `TestRunApp_StartsAndStops`.
- Verify : `go test ./internal/fx/... -run TestRunApp_StartsAndStops`
- Dépend de : M0b-saas-2

**M0b-saas-6 — README `--selfhosted` → `--unsafe`**
- Fichiers : `README.md`
- Changement : l.91-92 (exemple) et l.525 (table flags). **Ne pas toucher** la section 'SaaS Export' (24/54/485/493/740) ni le tag d'exemple `selfhosted` dans `examples/file-operations.yaml` (décision séparée).
- Verify : `grep -n "\-\-selfhosted\|selfhosted" README.md || true` (plus aucune ligne de flag)
- Dépend de : M0b-saas-4

**M0b-saas-7 — Réécrire commentaires SaaS → couture locale store-backed dans `export/port.go`**
- Fichiers : `internal/export/port.go`
- Changement : réécrire godoc package (1-8), `EventExporter` (17, drop 'remote sink' → 'noop in v1'), `IdempotencyClaimer` (26-28, dedup via UNIQUE store local, pas de GET_LOCK), `ExecutionRecoverer` (33-37, scan lock-free au boot), `RecoveredExecution` (50-55, l.53 'SaaS'→'store-backed recovery adapter'). Chaque godoc commence par son identifiant. Aucune signature modifiée. Zéro 'SaaS'/'remote' résiduel. **Couvre aussi le besoin M1a-0** (réécriture spec §4.2).
- Verify : `go build ./internal/export/... && go vet ./internal/export/...`
- Dépend de : —

**M0b-saas-8 — Réécrire commentaires SaaS dans `export/noop.go`**
- Fichiers : `internal/export/noop.go`
- Changement : godoc `NewNoopExporter` (5-6), `NewNoopClaimer` (9-13, fail-open by design, dedup réel via UNIQUE), `NewNoopRecoverer` (15-16). Corps inchangés. Zéro 'SaaS'.
- Verify : `go build ./internal/export/... && go vet ./internal/export/...`
- Dépend de : —

**M0b-saas-9 — Vérification globale footgun**
- Fichiers : `action_registry.go`, `module.go`, `cmd_serve.go`, `export/port.go`, `export/noop.go`
- Changement : grep repo entier 0 occurrence `SelfHosted|selfHosted|selfhosted|saasAllowedActions`. **Exceptions à conserver** : build tags `//go:build !saas` (exec/js/file/builtins_unsafe + `_test`), commentaires `saas` de `builtins.go` (9,13-14,56), section README 'SaaS Export', tag `selfhosted` de `examples/file-operations.yaml`, commentaire historique `exports_test.go`. Confirmer manuellement le signe `if !in.Config.Unsafe`.
- Verify : `go build ./... && go test ./internal/fx/... ./internal/export/... ./cmd/... && grep -rn "SelfHosted\|selfHosted\|saasAllowedActions" --include="*.go" . ; grep -rn "selfhosted" README.md cmd internal || true`
- Dépend de : M0b-saas-3,4,5,6,7,8

---

## JALON M1 — Idempotency & Recovery (bloquants v1)

> Préalable confirmé : `export` n'importe pas `store` ni `mariadb` (pas de cycle) ; `mariadb` peut importer `export` pour `*export.ClaimResult`. `fx` importe déjà `mariadb` et `parser`. `M1a-0` (réécriture commentaires export §4.2) est **absorbé par M0b-saas-7/8** — vérifier l'alignement (ajuster wording NewNoopClaimer pour préciser : noop acceptable seulement là où aucune dedup durable n'existe). Tâche M1a-0 = simple revue, pas de double édition.

### Idempotency (M1a)

**M1a-1 — (TDD) claim executionID == row ID persisté**
- Fichiers : `internal/server/handlers_test.go`, `internal/server/exports_test.go`
- Changement : test `HandlersTestSuite` pilotant `handlePublicTrigger` avec un `recordingClaimer` (NOUVEAU dans exports_test.go, ne pas muter `stubClaimer`) capturant l'`executionID` passé à `ClaimExecution`, renvoyant `ClaimResult{Claimed:true}`. Assert `ExecutionStore.Get(ctx,capturedID)` sans erreur. **Échoue actuellement** (handlers_triggers.go:83 claim un `uuid.New()`, :126 un autre).
- Test : `TestPublicTrigger_ClaimAndExecutionShareSameID`
- Verify : `go test ./internal/server/... -run 'TestHandlers/TestPublicTrigger_ClaimAndExecutionShareSameID'`
- Dépend de : —

**M1a-2 — Fix bug ID : générer executionID une fois et le propager (spec 3.1)**
- Fichiers : `internal/server/handlers_triggers.go`
- Changement : dans `executeTriggerWorkflow` générer `executionID := uuid.New().String()` UNE fois avant `handleIdempotencyCheck`. Signature `handleIdempotencyCheck` accepte `executionID` (le passe à `ClaimExecution` l.83, retirer le `uuid.New()` inline). `prepareTriggerExecution` accepte `executionID` (retirer `uuid.New()` l.126), garder le retour inchangé pour diff minimal. Seul appelant : `executeTriggerWorkflow`. Préserver le split assign/condition existant (78-79, 84).
- Tests : réutilise M1a-1 ; `..._IdempotencyDeduplicated` / `_IdempotencyClaimed` restent verts.
- Verify : `go build ./... && go test ./internal/server/...`
- Dépend de : M1a-1

**M1a-3 — (TDD) memory claimer : double claim même clé → Claimed=false**
- Fichiers : `internal/export/memory_claimer_test.go`
- Changement : nouveau fichier package `export`. 1er claim (exec-A,key-1)→Claimed=true ; 2e (exec-B,key-1)→Claimed=false, ExistingExecutionID==exec-A, ExistingStatus==`runtime.StatusRunning`. Clé OU workflow distinct→Claimed=true. Test concurrent N goroutines même (wf,key)→exactement un winner (`-race`).
- Tests : `TestMemoryClaimer_SecondClaimDeduplicates`, `_DistinctKeysClaim`, `_DistinctWorkflowsClaim`, `_ConcurrentSingleWinner`
- Verify : `go test -race ./internal/export/ -run TestMemoryClaimer`
- Dépend de : —

**M1a-4 — Memory claimer (spec 3.2)**
- Fichiers : `internal/export/memory_claimer.go`
- Changement : `NewMemoryClaimer() *MemoryClaimer`, `sync.Mutex` + `map[string]memoryClaim`, `claimKey = workflowName + "\x00" + idempotencyKey`. `ClaimExecution` : Lock, lookup ; présent → `ClaimResult{Claimed:false, ExistingExecutionID, ExistingStatus}` ; sinon store `{executionID, StatusRunning}` → `{Claimed:true}`. Commentaire : map non bornée acceptable v1. Godoc commence par l'identifiant.
- Verify : `go build ./... && go test -race ./internal/export/`
- Dépend de : M1a-3

**M1a-5 — Schéma MariaDB : colonne `idempotency_key` + UNIQUE**
- Fichiers : `internal/store/mariadb/schema.go`, `internal/store/mariadb/store_test.go`
- Changement : **PAS de table séparée** (spec 3.2). Dans le CREATE TABLE `%sexecutions` (schema.go:46) ajouter `idempotency_key VARCHAR(255) NULL` + `UNIQUE KEY uk_%sexec_idem (workflow_name, idempotency_key)` (NULLs exemptés en InnoDB). Garder 3 statements. `TestSchemaStatements_ContainsExpectedTables` (store_test.go:65) : Len==3, ajouter `s.Contains(stmts[0],"idempotency_key")` + `s.Contains(stmts[0],"uk_tf_exec_idem")` (testPrefix `tf_`).
- Tests : `TestSchemaStatements_ContainsExpectedTables`
- Verify : `go test ./internal/store/mariadb/ -run 'TestStore/TestSchemaStatements'`
- Dépend de : —
- **Open** : `CREATE TABLE IF NOT EXISTS` ne migre pas une table existante → ALTER guardé (voir §4 D1).

**M1a-6 — (TDD) claimer MariaDB sqlmock**
- Fichiers : `internal/store/mariadb/idempotency_test.go`
- Changement : suite sqlmock (`QueryMatcherRegexp`, `NewWithDB(db,"tf_")`). (1) clé fraîche → `ExpectExec INSERT INTO tf_executions ...` → `Claimed:true` ; (2) duplicate → `WillReturnError(&mysql.MySQLError{Number:1062,...})` puis `ExpectQuery SELECT id,status ... WHERE workflow_name=? AND idempotency_key=?` → `Claimed:false,Existing...` ; (3) autre erreur → propagée wrappée. Import `mysqldriver "github.com/go-sql-driver/mysql"`. `ExpectationsWereMet`.
- Tests : `TestClaimExecution_FreshKeyClaims`, `_DuplicateReturnsExisting`, `_OtherErrorPropagates`
- Verify : `go test ./internal/store/mariadb/ -run 'TestStore/TestClaimExecution'`
- Dépend de : M1a-5

**M1a-7 — Claimer MariaDB (spec 3.2 : INSERT = claim, 1062 → SELECT)**
- Fichiers : `internal/store/mariadb/idempotency.go`
- Changement : méthode `(*Store) ClaimExecution(ctx, executionID, workflowName, idempotencyKey string) (*export.ClaimResult, error)`. INSERT `(id, workflow_name, status, started_at, idempotency_key)` status=`StatusRunning`. Sur erreur : `errors.As(execErr,&myErr); if ok && myErr.Number==1062 { SELECT id,status ... → ClaimResult{Claimed:false,...} }` sinon `fmt.Errorf("mariadb claim: %w", execErr)`. Succès → `{Claimed:true}`. Pas de transaction/GET_LOCK ; UNIQUE = seule autorité. `var _ export.IdempotencyClaimer = (*Store)(nil)`. Godoc commence par `ClaimExecution`.
- Verify : `go build ./... && go test ./internal/store/mariadb/`
- Dépend de : M1a-5, M1a-6

**M1a-8 — (TDD) NewExportPorts sélectionne le bon claimer par backend**
- Fichiers : `internal/fx/export_ports_test.go`
- Changement : étendre `ExportPortsTestSuite`. nil/memory → `*export.MemoryClaimer` ; mariadb + `*mariadb.Store` (via `NewWithDB(sqlmock,"tf_")`) → ce Store ; renommer `..._AlwaysWiresNoops`→`..._ExporterRecovererStayNoop`. (ClickHouse supprimé en M0 → pas d'arm dédié.)
- Tests : `TestNewExportPorts_MemoryUsesMemoryClaimer`, `_MariaDBUsesStoreClaimer`, `_ExporterRecovererStayNoop`
- Verify : `go test ./internal/fx/ -run TestExportPorts`
- Dépend de : M1a-4, M1a-7

**M1a-9 — Câbler NewExportPorts (claimer par backend) + Add tolérant duplicate-key**
- Fichiers : `internal/fx/export_ports.go`, `internal/server/handlers_triggers.go`
- Changement : ajouter à `ExportPortsIn` : `Workflow *parser.Workflow`, `Store store.ExecutionStore` (déjà fournis par fx). Type-switch sur `in.Workflow.Persistence` : nil/memory → `NewMemoryClaimer()` ; mariadb → `in.Store.(*mariadb.Store)` (fallback défensif `NewMemoryClaimer`) ; défaut → `NewNoopClaimer()`. Exporter/Recoverer restent noop. **Fix cohérence** : le claimer MariaDB ayant INSERT la row, `prepareTriggerExecution`'s `Add` pour le même id frapperait l'UNIQUE/PK → rendre `Add` tolérant au duplicate-key (log debug, ne pas échouer). Documenter le couplage. Godoc commence par `NewExportPorts`.
- Tests : M1a-8 ; M1a-2 reste vert (memory n'INSERT pas via claimer).
- Verify : `go build ./... && go test ./internal/fx/ ./internal/server/...`
- Dépend de : M1a-8, M1a-2

**M1a-10 — (TDD e2e) double trigger même clé → 1 exécution**
- Fichiers : `internal/server/handlers_test.go`
- Changement : câbler le **vrai** `export.NewMemoryClaimer()` dans `newTestServerIdempotent` (accepte déjà un `IdempotencyClaimer`, handlers_test.go:2208). Deux `handlePublicTrigger` même body `{"order_id":"ord-e2e"}`. 1er → exec créée, capter execution_id ; 2e → deduplicated==true, même id, `ExecutionStore.Count` inchangé (==1).
- Test : `TestPublicTrigger_SameKeyTwiceSingleExecution`
- Verify : `go test ./internal/server/... -run 'TestHandlers/TestPublicTrigger_SameKeyTwiceSingleExecution'`
- Dépend de : M1a-2, M1a-4

### Recovery (M1b/M1c/M1d/M1e)

**M1cov-noop — (filet) couverture `internal/export` (0.0% → ports noop + DTO)**
- Fichiers : `internal/export/noop_test.go`
- Changement : suite testify. `NewNoopExporter().Start/Shutdown` sans panic ; `NewNoopClaimer().ClaimExecution`→`{Claimed:true},nil` ; `NewNoopRecoverer().RecoverExecutions`→`(nil,nil)` ; table `json.Unmarshal(RecoveredExecution)` peuple `RawParams`/`RawSteps`, `Params`/`Steps` (`json:"-"`) restent nil. Split err.
- Tests : `TestNoopExporter_StartShutdownNoPanic`, `TestNoopClaimer_AlwaysGrants`, `TestNoopRecoverer_YieldsNothing`, `TestRecoveredExecution_JSONBoundary`
- Verify : `go test -cover ./internal/export/...`
- Dépend de : —

**M1cov-mariadb — (filet) couverture `internal/store/mariadb` (68.5% → cible >=85%)**
- Fichiers : `internal/store/mariadb/store_test.go`
- Changement : étendre `StoreTestSuite`. `Update` succès + erreur driver wrappée 'mariadb update' ; `encodeJSON` (nil/typed-nil/empty → NULL, non-vide → JSON) ; `UpdateStep` dérive 'waiting' (BeginTx + lock SELECT...FOR UPDATE + writeExecutionTx + Commit) ; `UpdateExecution` write-back error → Rollback ; pagination events `events.go` (pas la méthode MemoryExecutionStore) ; `StepExecCounts` scan error ; `scanExecution` decode params invalide → 'decode params'. **Plafond réaliste** : New()/migrate Ping non atteignable via sqlmock — reporter le chiffre atteint + lignes non couvertes si 85% inatteignable.
- Verify : `go test -cover ./internal/store/mariadb/...`
- Dépend de : —

**M1b-1 — (TDD) Memory recoverer : scan lock-free sur MemoryExecutionStore**
- Fichiers : `internal/store/memory_recoverer.go`, `internal/store/memory_recoverer_test.go`
- Changement : package `store` (pas de cycle, `export` n'importe pas `store`). `MemoryRecoverer` wrappe `*MemoryExecutionStore`, implémente `export.ExecutionRecoverer`. `RecoverExecutions(ctx,agentID)` appelle `s.store.List(ctx)`, filtre Status ∈ {Running,Waiting}, map vers `export.RecoveredExecution{ExecutionID:e.ID, WorkflowName, Status, Params:e.Params, Steps:e.Steps}`. `agentID` ignoré (mono-instance). `NewMemoryRecoverer(s)`. Godoc commence par l'identifiant. Split err.
- Tests (FIRST) : `TestMemoryRecoverer_ReturnsRunningAndWaiting`, `_EmptyStoreYieldsNothing`, `_PreservesNewestFirst`
- Verify : `go build ./... && go test ./internal/store/...`
- Dépend de : M1cov-noop

**M1b-2 — (TDD) MariaDB recoverer : SELECT status IN ('running','waiting')**
- Fichiers : `internal/store/mariadb/recoverer.go`, `internal/store/mariadb/recoverer_test.go`
- Changement : méthode `(*Store) RecoverExecutions(ctx,agentID) ([]export.RecoveredExecution,error)`. **Projection EXACTE 8 colonnes** que `scanExecution` attend : `SELECT id, workflow_name, status, params, steps, started_at, finished_at, error_msg FROM %sexecutions WHERE status IN (?,?) ORDER BY created_seq ASC` — `created_seq` UNIQUEMENT dans ORDER BY, **jamais dans SELECT**. Args `StatusRunning, StatusWaiting`. Réutiliser `scanExecution`. `agentID` ignoré. Wrap 'mariadb recover: ...'. Split err. Godoc `RecoverExecutions`.
- Tests (FIRST, sqlmock) : `TestRecoverExecutions_ReturnsNonTerminal` (WithArgs(Running,Waiting), **pin la projection 8 colonnes + ORDER BY created_seq ASC** par regexp), `_EmptyResult`, `_QueryError`, `_ScanDecodeError`
- Verify : `go test -cover ./internal/store/mariadb/...`
- Dépend de : M1cov-noop, M1cov-mariadb

**M1c-1 — Câbler les recoverers dans NewExportPorts (backend-aware)**
- Fichiers : `internal/fx/export_ports.go`, `internal/fx/export_ports_test.go`
- Changement : utiliser `Store`/`Workflow` déjà ajoutés à `ExportPortsIn` (M1a-9). Quand `in.Workflow.Recovery==true` : type-switch `in.Store` — `*mariadb.Store`→Recoverer direct ; `*store.MemoryExecutionStore`→`store.NewMemoryRecoverer(s)` ; défaut→`NewNoopRecoverer()`. Claimer/Exporter restent câblés comme en M1a-9. **Pas d'arm clickhouse** (supprimé M0, défaut→noop suffit). MAJ godoc package (recoverer store-backed).
- Tests : `TestNewExportPorts_MemoryBackendRecovererWhenEnabled`, `_NoopWhenRecoveryDisabled`, `_ClaimerExporterAlwaysNoop` ; `internal/fx/module_test.go` valide le graphe fx.
- Verify : `go build ./... && go test ./internal/fx/...`
- Dépend de : M1b-1, M1b-2, M1a-9

**M1d-1 — (TDD pinning) handleRecovery : gap StatusWaiting fall-through**
- Fichiers : `internal/engine/engine_test.go`
- Changement : skip/fail/retry/non-running DÉJÀ couverts (ne pas y toucher, notamment le retry-default :1750 qui assert déjà la ré-exécution). Vrai gap : engine.go:555 `if sr.Status != runtime.StatusRunning { return false,nil }` → un step recovered en `StatusWaiting` re-exécute sans special-case. Ajouter `TestExecute_RecoveryWaitingStepFallsThrough` (seed waiting, assert non-skippé, `StepStarted` émis). Optionnel `TestExecute_RecoveryMixedGraphResumesOnce`. Code prod modifié SEULEMENT si un test révèle un vrai défaut (alors petit helper, ne pas gonfler engine.go).
- Tests : `TestExecute_RecoveryWaitingStepFallsThrough` (+ mixed si non redondant)
- Verify : `go test ./internal/engine/...`
- Dépend de : —

**M1e-1 — (TDD e2e) recovery mémoire complète**
- Fichiers : `internal/server/recovery_e2e_test.go`
- Changement : package `server`, sans DB réelle. MemoryExecutionStore pré-seedé (`Add`) avec une exec `running` matchant `s.config.Workflow.Name`, step success + step running mappé `on_recovery=skip` ; `config.Recoverer = store.NewMemoryRecoverer(store)` ; `config.Workflow.Recovery=true`. Appeler `srv.recoverExecutions(ctx)`. Subscribe EventBus : `WorkflowStarted` pour l'exec recovered, step success NON re-émis, step `on_recovery=skip` NON ré-exécuté, downstream tourne, terminal success. 2e cas `TestRecovery_E2E_OnlyNonTerminalResumed` (running+success+failed → seul running repris). Préférer le vrai `MemoryRecoverer` (garder `stubRecoverer` pour l'injection d'erreur).
- Tests : `TestRecovery_E2E_MemoryBackend_ResumesWithOnRecoverySkip`, `_OnlyNonTerminalResumed`
- Verify : `go test ./internal/server/... && go test ./...`
- Dépend de : M1b-1, M1c-1, M1d-1

**M1a-11 — Gate M1 (build + race + lint Iliad)**
- Fichiers : tous les fichiers M1 nouveaux/modifiés
- Changement : 0 assign-in-if (`grep -nE 'if [a-zA-Z_]+ ?:?= .*; .* (!=|==) '` vide sur fichiers touchés) ; chaque godoc exporté commence par son nom ; commentaires anglais ; tous < 500 l (`wc -l`).
- Verify : `go build ./... && go test -race ./... && (golangci-lint run ./... 2>/dev/null || true)`
- Dépend de : M1a-0(absorbé), M1a-2, M1a-7, M1a-4, M1a-9, M1a-10, M1c-1, M1e-1

---

## JALON M2 — Finitions (UI/SSE + découpe + docs)

### Décision UI (bloquante en tête de M2)

**M2a-0 — Trancher la décision UI et figer la branche**
- Fichiers : `docs/CLEANUP_AUDIT.md`, spec
- Changement : état réel — `StepTimeline.vue` ABSENT ; `WorkflowDAGCustom.vue` PRÉSENT, importé par `ExecutionView.vue` (12/376), `EditorView.vue` (6/572), `DashboardView.vue` (13/377) ; `main.ts` route `/,/executions,/executions/:id,/docs,/editor`. **BRANCHE A** (recommandée) : acter WorkflowDAGCustom viz officielle, resync doc seule. **BRANCHE B** : recâbler StepTimeline. Aucune tâche UI avant fixation.
- Verify : `grep -rln 'WorkflowDAGCustom' web/frontend/src/ ; find web/frontend/src -name 'StepTimeline*' || echo absent`
- Dépend de : —

**M2a-A1 — [BRANCHE A] Acter WorkflowDAGCustom dans CLEANUP_AUDIT.md**
- Fichiers : `docs/CLEANUP_AUDIT.md`
- Changement : retirer 'StepTimeline à câbler' / 'WorkflowDAGCustom KILL' ; acter viz officielle + suppressions DONE.
- Verify : `for c in StepTimeline WorkflowGraph.vue StepNode.vue ExecutionList.vue StepDetailView.vue WorkflowView.vue; do find web/frontend/src -name "$c" | grep . || echo "$c absent (DONE)"; done`
- Dépend de : M2a-0
- *(BRANCHE B alternative : M2a-B1 créer StepTimeline.vue + spec Vitest ; M2a-B2 recâbler ExecutionView/DashboardView, garder DAG dans EditorView. Activées seulement si M2a-0 choisit B.)*

### Fix goto-reset SSE

**M2b-1 — (TDD) engine : signal goto-reset fiable (réutiliser `event.StepGoto`/`Data['body']`)**
- Fichiers : `internal/engine/goto_test.go`, `internal/engine/goto.go`
- Changement : `publishGotoEvent` (goto.go:129-145) émet déjà `event.StepGoto` avec `Data['body']`. Module `tailflow-shared` v0.4.0 épinglé → **réutiliser StepGoto** (les 3 consommateurs le parlent déjà). Durcir le contrat : (1) garantir `Data['body']` []string non-nil contenant tous les ids reset ; (2) invariant d'ordre : StepGoto observé avant tout step.started d'itération 2 ; (3) godoc `publishGotoEvent` documentant que `Data['body']` est la liste de reset autoritative. Pas de changement de comportement au-delà du godoc.
- Tests (FIRST) : `TestGotoEvent_BodyIsResetList` (boucle work→save, StepGoto.Data['body']=={work,save}, précède le 2e step.started), `TestGotoEvent_BodyNeverNil`
- Verify : `go test ./internal/engine/ -run TestGoto -count=1 && go test ./internal/engine/ -cover`
- Dépend de : M0a-2 (assign-in-if déjà splitté dans engine cluster)

**M2b-2 — Frontend : handler SSE `step.goto` reset les statuts des body steps**
- Fichiers : `web/frontend/src/composables/useSSE.ts`
- Changement : vocabulaire réel = `running|success|failed|skipped|waiting` (pas 'ok'). Le badge stale est `success` ; cible reset `pending` (parité `handlers_capture.go:107` + fallback `WorkflowDAGCustom.vue:202`). Dans le listener `step.goto` (255-266) : en plus de `pendingIterations[bid]=iteration`, set `pendingStatuses[bid]='pending'` pour chaque `bid` de `event.data.body`, pour que `flushBatch` écrase le badge stale. Garder le batching. `handleSSE` n'applique PAS le filtre InLoop → reset sur step.goto nécessaire et suffisant côté live.
- Tests : **pas de runner FE** (package.json: dev/build/preview, pas de vitest). Défaut **Option B** : vérification manuelle documentée (les badges body passent au gris/pending à chaque itération). Option A (ajouter vitest) seulement si explicitement souhaité — ne pas prétendre un test auto inexistant.
- Verify : `cd web/frontend && npm run build`
- Dépend de : M2b-1

### Découpe engine.go (séquentiel, relocations pures)

**M2b-3 — Extraire result/status/params/env → `engine_result.go`**
- Fichiers : `internal/engine/engine.go`, `internal/engine/engine_result.go`
- Changement : relocation verbatim de `ExecuteResult`(53), `ExecuteOptions`(63), `resolveExecutionID`(213), `buildExecutionContext`(221), `buildExecuteResult`(237), `publishWorkflowCompleted`(256), `copyStepResults`(266), `resolveParams`(276), `resolveEnv`(350), `determineStatus`(368), `emitExecutionState`(149), `emitInitialExecutionState`(184). Garder `Executor`/`NewExecutor`/`Execute`. Le comma-ok `if _, ok := params[p.Name]; ok` (299) est toléré, laisser tel quel.
- Tests : aucun nouveau ; suite existante (98.6%) + `-race`.
- Verify : `go build ./... && go test ./internal/engine/ -race -count=1 -cover`
- Dépend de : M2b-1

**M2b-4 — Extraire DAG scheduling → `engine_dag.go`**
- Fichiers : `internal/engine/engine.go`, `internal/engine/engine_dag.go`
- Changement : relocation de `loopInfo`(384), `dagState`(390), `newDAGState`(404), `executeDAG`(441), `dagLoop`(452), `skipNode`(472), `runDAGNode`(490), `enqueueChildren`(528). **Ne pas** toucher `DAGNode/DAG/BuildDAG` (déjà dans `dag.go`) ni `handleGoto/resetLoopBody` (dans `goto.go`). Pas de collision avec `dag.go`.
- Verify : `go build ./... && go test ./internal/engine/ -race -count=1 -cover`
- Dépend de : M2b-3

**M2b-5 — Extraire exécution single-node + retry/timeout → `engine_node.go` (+ `engine_retry.go`)**
- Fichiers : `internal/engine/engine.go`, `internal/engine/engine_node.go`
- Changement : relocation de `handleRecovery`(541)…`stepError`(1204) (~670 l). **`engine_node.go` dépassera 500 l → split MANDATOIRE** : créer `engine_retry.go` pour `executeWithRetry`/`waitForRetry`/`applyTimeout`(913-1009).
- Verify : `go build ./... && go test ./internal/engine/ -race -count=1 -cover && wc -l internal/engine/engine_node.go internal/engine/engine_retry.go`
- Dépend de : M2b-4

**M2b-6 — Extraire test-case mock/expect → `engine_testcase.go`**
- Fichiers : `internal/engine/engine.go`, `internal/engine/engine_testcase.go`
- Changement : relocation de `findTestCase`(1215), `applyTestMock`(1225), `CheckTestExpectExposed`(1258, exporté), `checkTestExpect`(1262), `partialMatch`(1295), `scalarEqual`(1339). Ce cluster **contient les 3 assign-in-if 1273/1309/1325**. Si M0a-2 a déjà landé → relocation verbatim. Sinon fix-on-move. `engine_testcase.go` doit avoir 0 assign-in-if. Après les 4 splits, engine.go < 300 l, tous `engine_*.go` < 500 l.
- Verify : `go build ./... && go test ./internal/engine/ -race -count=1 -cover && wc -l internal/engine/*.go | sort -n && ! grep -nE 'if [a-zA-Z_]+ :=.*; .*!= nil' internal/engine/engine_testcase.go`
- Dépend de : M2b-5, M0a-2

### Découpe server.go

**M2b-7 — `server.go` (504 l) : split léger → `server_lifecycle.go`**
- Fichiers : `internal/server/server.go`, `internal/server/server_lifecycle.go`
- Changement : server.go garde `Config`, `Server`, fn vars, `New/Run/Handler/WaitRegistry`, registres cancel/timer. Déplacer `startExporter`, `recoverExecutions`(162, scan lock-free, NO GET_LOCK), `startMetricsRefresh`+`stepMetricsRefresher`, `startCronScheduler`, `startRabbitMQConsumer`, `shutdownServices`, `asyncRunOpts`, `runWorkflowAsync`(200), `ensureWorkflowCompleted`. Relocation verbatim, imports splittés. Ne pas re-toucher le modèle idempotency/recovery. Les deux fichiers < 500 l.
- Verify : `go build ./... && go test ./internal/server/ -race -count=1 && wc -l internal/server/server.go internal/server/server_lifecycle.go`
- Dépend de : —

### Docs (parallèles)

**M2c-1 — Resync CLEANUP_AUDIT.md ↔ STATUS.md**
- Fichiers : `docs/CLEANUP_AUDIT.md`, `web/frontend/docs/STATUS.md`
- Changement : retirer de STATUS.md 'Files of interest' les composants inexistants (`StepDetailView.vue`:111, `StepNode.vue`:116). Acter purge des morts + split `handlers.go` DONE. Synchroniser la liste réelle.
- Verify : `for f in $(grep -oE '[A-Za-z][A-Za-z0-9_/.-]*\.(vue|ts)' web/frontend/docs/STATUS.md | sort -u); do find web/frontend/src -name "$(basename $f)" | grep -q . && echo "OK $f" || echo "CHECK $f"; done`
- Dépend de : M2a-0

**M2c-2 — Resync README 'SaaS Export' → roadmap/non dispo v1 (spec §4.3)**
- Fichiers : `README.md`
- Changement : ancre TOC `#saas-export` (l.24) sans heading. Créer une courte section 'SaaS Export (roadmap, non disponible en v1)' OU retirer l'ancre. MAJ bloc Architecture (485 'SaaS Exporter optional', 493 'Non-blocking export') : EventExporter reste noop v1, ports idempotency/recovery implémentés localement store-backed.
- Verify : `grep -n 'saas-export' README.md; grep -n 'SaaS Export' README.md || echo 'no heading'`
- Dépend de : —

**M2c-3 — Resync EXPORT_COUPLING_AUDIT.md (post-Path-B + couture dormante)**
- Fichiers : `docs/EXPORT_COUPLING_AUDIT.md`
- Changement : acter Path B implémenté, lister call-sites actuels (`handlers_triggers.go:83` Claimer, `server.go:169` Recoverer, `server.go:159` Exporter), code SaaS réel retiré. Préciser : interfaces `export.*` conservées (couture dormante), implémentées store-backed en M1 ; seul EventExporter reste noop v1.
- Verify : `ls internal/export/; find internal -name exporter.go -o -name claim.go -o -name recovery.go | grep -v node_modules || echo 'no legacy saas impl OK'`
- Dépend de : —

---

## 4. Décisions ouvertes (à trancher, avec moment)

| # | Décision | Moment | Recommandation |
|---|---|---|---|
| D1 | **Stratégie schéma MariaDB pour installs existantes** : `CREATE TABLE IF NOT EXISTS` ne migre pas → ALTER guardé (ADD COLUMN/ADD UNIQUE tolérant duplicate-error, MySQL<8.0/MariaDB<10.0.2 sans `IF NOT EXISTS`) vs idempotency fresh-install-only | Avant M1a-5 | Ajouter ALTER guardé dans `migrate()` |
| D2 | **Propriétaire de l'INSERT de la row pour triggers MariaDB keyed** : claimer (spec 3.2) vs `prepareTriggerExecution.Add` | M1a-9 | Claimer crée la row ; `Add` tolérant duplicate-key |
| D3 | **Branche UI** : A (WorkflowDAGCustom officiel, doc only) vs B (recâbler StepTimeline) | Tête de M2 (M2a-0, **bloquant**) | Branche A |
| D4 | **server.go 504 l** : split léger vs exemption documentée | M2b-7 | Split léger (`server_lifecycle.go`) |
| D5 | **Découpe engine.go** : fichiers thématiques (`engine_result/dag/node/retry/testcase`) en préservant 98.6% | M2b-3..6 | Découpe ci-dessus, séquentielle |
| D6 | **README SaaS Export** : section roadmap courte vs retrait pur de l'ancre | M2c-2 | Section roadmap courte |
| D7 | **Harness de test FE** : Option A (ajouter vitest) vs B (vérif manuelle) | M2b-2 | Option B (garder le scope) |
| D8 | **Cible reset client** : `pending` vs `running` | M2b-2 | `pending` (parité refresh) |
| D9 | **Ordonnancement M0a-2 vs M2b-6** : land M0 d'abord (relocation verbatim) vs fix-on-move | M2b-6 | Land M0 d'abord (dépendance déclarée) |
| D10 | **ExistingStatus freshness** : capturé au claim (memory) vs re-SELECT live (mariadb) | M1a-4/7 | Accepter l'asymétrie v1 |
| D11 | **Nom fonction renommée** : `safeAllowedActions` vs `defaultAllowlist` | M0b-saas-3 | `safeAllowedActions` |

**Hors scope (post-v1, tracé)** : TTL/cap de la map MemoryClaimer ; rétention/cleanup des rows d'idempotency ; ClickHouse comme sink analytics alimenté depuis MariaDB ; HA/multi-instance/ownership-recovery ; section README SaaS Export marketing complète ; docs `2026-03-10-persistence-saas-*` historiques.

---

## 5. Risques & garde-fous (consolidés)

1. **[SÉCU — critique] Inversion `!SelfHosted`→`!Unsafe`** : garder le `!`. Une erreur de signe ouvrirait exec/js/file.* out-of-the-box. Garde-fous : M0b-saas-1 (red verrouille `Create("exec")`→error quand Unsafe=false) + M0b-saas-9 (re-vérif manuelle du signe). La couverture 2-branches vit dans `action_registry_test.go`, pas `module_test.go`.
2. **[Build cassé transitoire] CH-2→CH-1** : ne builder qu'après CH-1. Idem coupling parser : CH-4 après CH-5+CH-6.
3. **[Design 3.2] Claim = INSERT de la row** : claimer MariaDB et `Add` se disputent le même id → M1a-9 rend `Add` tolérant au duplicate-key. Sinon le happy-path keyed échoue au 2e insert.
4. **[Migration schéma] CREATE TABLE IF NOT EXISTS ne migre pas** (D1) : fresh installs OK ; existants nécessitent ALTER.
5. **[Projection colonne — critique] M1b-2** : SELECT EXACTEMENT 8 colonnes (`scanExecution`), `created_seq` UNIQUEMENT en ORDER BY. Pin par assertion regexp pour qu'une dérive d'ordre fasse échouer le test.
6. **[sqlmock duplicate-key]** : retourner un vrai `*mysql.MySQLError{Number:1062}` pour que `errors.As` matche la branche dedup.
7. **[Mono-instance lock-free]** : 2 agents rejoueraient la même exec si le déploiement devenait multi-instance. Documenté dans le godoc recoverer. Idempotency = contrainte UNIQUE, pas de claim distribué, pas de GET_LOCK (aucun dans `internal/`). HA = post-v1.
8. **[engine_node.go > 500 l CERTAIN]** : split `engine_retry.go` mandatoire (M2b-5), gate `wc -l`.
9. **[engine.go fichier chaud]** : M0a-2 (M0) avant M2-SSE/M2b-* ; séquencer M2b-1→M2b-3→…→M2b-6 ; ne pas dupliquer DAG/goto déjà externalisés.
10. **[Module épinglé]** : `tailflow-shared` v0.4.0, pas de nouveau EventType partagé → réutiliser `event.StepGoto`/`Data['body']`.
11. **[FE pas de runner]** : ne pas prétendre un test vitest inexistant (D7) ; vocabulaire `success`/`pending` correct.
12. **[testify/suite]** : tests = méthodes sur la suite, pas `func TestX(t *testing.T)`. Nouveaux cas = méthodes avec `s.Require()/s.Contains`, split assign/condition.
13. **[go mod tidy]** : revoir le diff (seuls modules ClickHouse retirés, `go-sql-driver/mysql` conservé) ; nécessite cache/réseau module.
14. **[Faux positifs grep UI]** : chercher des noms de fichiers exacts (`find -name X.vue`), pas des sous-chaînes (StepNode matcherait StepInspector).
15. **[Iliad transverse]** : 0 assign-in-if, godoc commence par l'identifiant, commentaires anglais, < 500 l, TDD test-first. Gates grep dans M0a-3, M1a-11, M2b-6.

---

## 6. Definition of Done par jalon

### DoD M0
- `go build ./... && go vet ./... && go test ./...` **vert**.
- `! grep -rIi clickhouse --include='*.go' . && ! grep -qi clickhouse go.mod && ! grep -qi clickhouse go.sum`.
- 0 occurrence `SelfHosted|selfHosted|saasAllowedActions` en `*.go` (hors exceptions : build tags `!saas`, commentaires historiques).
- 0 assign-in-if dans `handlers_wf.go` et `engine.go` (lignes 85/90/1273/1309/1325).
- Flag `--unsafe` opérationnel ; `Create("exec")` bloqué par défaut (`Unsafe=false`), autorisé avec `--unsafe`. README sans `--selfhosted`.
- godoc `ScheduleTrigger`/`ScheduleAction`/`NewScheduleAction` clarifiés ; mot-clé YAML `schedule` inchangé.
- Commentaires `export/port.go`+`noop.go` réécrits couture locale store-backed (0 'SaaS'/'remote').

### DoD M1
- `go build ./... && go test -race ./...` **vert**.
- **Critère fonctionnel idempotency** : double trigger même clé → **exactement 1 exécution** (`TestPublicTrigger_SameKeyTwiceSingleExecution` : deduplicated==true, même execution_id, `Count`==1). claim executionID == row ID persisté (`TestPublicTrigger_ClaimAndExecutionShareSameID`).
- Claimer réel câblé par backend : memory→`MemoryClaimer`, mariadb→`*mariadb.Store` (UNIQUE `uk_*_exec_idem`), autre→noop. `var _ export.IdempotencyClaimer = (*mariadb.Store)(nil)` compile.
- **Critère fonctionnel recovery** : exec `running`/`waiting` au boot → reprise via scan lock-free (`TestRecovery_E2E_MemoryBackend_ResumesWithOnRecoverySkip`) ; seules les non-terminales reprises (`_OnlyNonTerminalResumed`) ; step `on_recovery=skip` non ré-exécuté ; gap StatusWaiting verrouillé (`TestExecute_RecoveryWaitingStepFallsThrough`).
- Couverture : `internal/export` > 0% (ports noop + DTO) ; `internal/store/mariadb` ≥ 85% ou chiffre atteint + lignes non couvertes (New/migrate Ping) reportées.
- 0 assign-in-if + godoc nommés + fichiers < 500 l sur les fichiers M1.

### DoD M2
- `go build ./... && go test -race ./...` **vert** ; couverture engine ≥ 98.6% (non-régression).
- **Critère fonctionnel SSE** : workflow goto bouclant émet `event.StepGoto` avec `Data['body']` non-nil = liste reset, avant le 2e `step.started` (`TestGotoEvent_BodyIsResetList`, `_BodyNeverNil`) ; côté FE les badges body passent à `pending` à chaque itération (vérif build + manuelle).
- **Tous** les `internal/engine/*.go` et `internal/server/server.go`/`server_lifecycle.go` < 500 l (`wc -l`) ; engine.go < 300 l.
- `engine_testcase.go` : 0 assign-in-if (grep gate).
- `web/frontend && npm run build` **vert**.
- Décision UI figée (M2a-0) ; docs synchronisés : STATUS.md sans composant fantôme, README sans ancre orpheline, EXPORT_COUPLING_AUDIT.md à jour (Path B implémenté, couture dormante).
