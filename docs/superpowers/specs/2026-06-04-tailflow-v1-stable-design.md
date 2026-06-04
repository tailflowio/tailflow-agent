# Design — tailflow-agent v1 stable (self-hosted, sans SaaS)

> Statut : design validé (brainstorming) — 2026-06-04
> Cible : première version stable de l'agent, self-hosted uniquement. Le SaaS arrive plus tard.

## 1. Objectif

Sortir une **première version stable** de `tailflow-agent` :

- **Fonctionnelle** : tout ce qui est exposé tient ses promesses (pas de faux-semblant).
- **Self-hosted only** : le SaaS est repoussé ; on neutralise ses coutures sans brûler les ponts.
- **Mono-instance** : un process agent par workflow. La HA multi-réplicas est post-v1.

Deux jalons de contenu : **M1 = « avoir les choses fonctionnel »**, **M2 = « à finir pour trancher »**, précédés d'un **M0 = sécuriser & dégraisser**.

## 2. Décisions de cadrage (figées)

| # | Sujet | Décision |
|---|-------|----------|
| D1 | Périmètre persistance | **Phase 4 (idempotency + recovery) bloquante** pour la v1 |
| D2 | Backends Phase 4 | **memory + MariaDB** réels. ClickHouse exclu |
| D3 | ClickHouse | **Retiré entièrement** du code (récupérable via commit `abf72a0`) |
| D4 | Périmètre v1 hors Phase 4 | Drift UI + comportements incertains + couverture + conventions = **tous in-scope** |
| D5 | Découplage SaaS | Supprimer le concept `SelfHosted` ; **allowlist secure-by-default** ; flag **`--unsafe`** (défaut `false`) ; build tag `saas` gardé en couture dormante ; interfaces `export.*` gardées en couture dormante (impls store-backed) |
| D6 | Modèle sécu actions | **Flag `--unsafe` global** : `exec`/`js`/`file.*` bloqués par défaut, libérés explicitement |
| D7 | Déploiement | **Mono-instance**. Pas de `GET_LOCK`, pas de Locker MariaDB. HA post-v1 |

## 3. Conception détaillée — Phase 4 (cœur du « fonctionnel »)

La tuyauterie existe déjà ; il manque les implémentations réelles. On remplace les noop fail-open par du **store-backed**, **derrière les interfaces `export.*` existantes** (couture dormante pour le retour du SaaS ; commentaires « remote authority / SaaS » réécrits en « backed by the local ExecutionStore »).

### 3.1 Fix préalable — bug d'ID d'exécution

`handleIdempotencyCheck` claim avec `uuid.New()` (`handlers_triggers.go:83`) puis `prepareTriggerExecution` génère un **autre** `uuid.New()` (`:126`). L'ID claimé ≠ l'ID exécuté.

**Correction** : générer l'`executionID` **une seule fois** en amont et le passer au claim *et* à l'exécution.

### 3.2 Idempotency claim — contrainte UNIQUE (pas de `GET_LOCK`)

L'idempotency exige un enregistrement **durable** ; `GET_LOCK` est transitoire (auto-relâché) donc inadapté.

- **MariaDB** : colonne `executions.idempotency_key` + `UNIQUE (workflow_name, idempotency_key)`.
  - Claim atomique = `INSERT` de la ligne d'exécution avec sa clé.
  - Sur violation de duplicate key → `SELECT` l'exécution existante → `ClaimResult{Claimed:false, ExistingExecutionID, ExistingStatus}`.
  - Réutilise l'infra transactionnelle déjà en place (`BeginTx`, et `lockExecution`/`FOR UPDATE` si une lecture cohérente est requise).
- **Memory** : `map[claimKey]execInfo` sous mutex (`claimKey = workflowName + "\x00" + idempotencyKey`), check-and-set atomique.
- `ClaimExecution(ctx, executionID, workflowName, idempotencyKey)` conserve sa signature actuelle (`export/port.go:30`).

### 3.3 Recovery — scan lock-free au boot (mono-instance)

En mono-instance, au démarrage un seul agent existe : aucune coordination nécessaire.

- **MariaDB** : `SELECT … FROM executions WHERE status IN ('running','waiting')` → hydrate `[]RecoveredExecution` (params + steps).
- **Memory** : itère le store in-process pour les statuts non-terminaux.
- La relance existe déjà : `server.recoverExecutions` (`server.go:162`) → `runWorkflowAsync{Resumed:true, RecoveredSteps}` (`:200`). On branche le `Recoverer` réel sur `NewExportPorts`.
- Garde `if !s.config.Workflow.Recovery { return }` (opt-in workflow déjà présent).

### 3.4 Consommation de `step.on_recovery`

`engine.handleRecovery` (`engine.go:541`) existe déjà. **Vérifier et tester** qu'il applique bien `retry`/`skip`/`fail` sur l'exécution reprise selon l'état des steps recouvrés.

### 3.5 Filet de sécurité (préalable à 3.2–3.4)

Remonter la couverture **avant** d'adosser la logique Phase 4 :

- `internal/store/mariadb` (≈68 % → cible ≥85 %).
- `internal/export` (actuellement sans test).
- Tests **e2e** idempotency (double trigger même clé → 1 exécution) et recovery (exécution `running` au boot → reprise).

## 4. Décommission SaaS

### 4.1 Footgun allowlist / `SelfHosted` (D5, D6)

État actuel : `--selfhosted` défaut `false` → l'allowlist SaaS bloque `exec`/`js`/`file.*` au runtime out-of-the-box (`fx/action_registry.go:24`, `cmd_serve.go:35`).

Cible :

- **Supprimer le concept `SelfHosted`** (`fx/module.go:24`, `cmd_serve.go`, `fx/action_registry.go`).
- **Allowlist appliquée par défaut** (secure-by-default).
- Nouveau flag **`--unsafe`** (défaut `false`) : quand activé, désactive l'allowlist (toutes les actions compilées sont runnable).
- Renommer le test `TestNewActionRegistry_DefaultAppliesSaaSAllowlist` → sémantique `unsafe` ; conserver la couverture des deux branches.

### 4.2 Couture dormante (conservée, non exercée)

- **Build tag `saas`** (`//go:build !saas` sur `exec`/`js`/`file`/`builtins_unsafe`) : conservé comme durcissement compile-time futur. Le build par défaut reste `!saas` (tout inclus).
- **Interfaces `export.IdempotencyClaimer` / `ExecutionRecoverer` / `EventExporter`** : conservées. `EventExporter` reste noop en v1. Réécrire les commentaires « SaaS / remote authority » des fichiers `export/port.go` et `export/noop.go` pour refléter l'implémentation locale store-backed.

### 4.3 Docs SaaS

- README : section « SaaS Export » → marquer **roadmap / non disponible en v1** (ou déplacer en note roadmap).
- `docs/EXPORT_COUPLING_AUDIT.md` : aligner sur l'état réel post-purge.

## 5. Finitions (M2) — « à finir pour trancher »

### 5.1 Drift UI

`docs/CLEANUP_AUDIT.md` décrit un état basé sur `StepTimeline` (absent) et la purge de `WorkflowDAGCustom` (toujours utilisé par 3 vues).

**Trancher** : soit exécuter la purge documentée (`StepTimeline` recâblé, `WorkflowDAGCustom` supprimé), soit acter l'abandon. Puis **resync** `CLEANUP_AUDIT.md` ↔ `STATUS.md`. *(Décision UI à prendre en début de M2.)*

### 5.2 Comportements incertains

- **`schedule` action vs `Trigger.Schedule`** : ce ne sont **pas** des doublons (`internal/action/schedule.go` = action de step delay/at, server-mode ; `Trigger.Schedule` = cron déclencheur). Action : **clarifier nommage / doc**, pas de suppression.
- **goto reset event non émis en SSE** : un reset de boucle `goto` ne pousse pas d'event SSE → état visuel stale dans l'UI. Action : **émettre l'event de reset** côté engine/SSE.

### 5.3 Conventions Go

- **5 assign-in-if** : `handlers_wf.go:85,90` ; `engine.go:1273,1309,1325` → scinder assignation et condition (règle Iliad).
- **`engine.go` (1341 l)** : découper en fichiers thématiques, **avec filet de tests** (engine ≈98.6 % — préserver).
- **`server.go` (504 l)** : statuer (découpe légère ou exemption `_test.go` de la règle 500 l).

## 6. Séquencement — 3 jalons

### M0 · Sécuriser & dégraisser (rapide, faible risque)
1. **Push les 18 commits** `develop → origin/develop` (sécurise le travail local).
2. Retirer ClickHouse entièrement (`internal/store/clickhouse`, wiring fx, enum backend, schema, tests ; nettoyer le commentaire ClickHouse `server.go:209`).
3. Décommission SaaS footgun : supprimer `SelfHosted`, ajouter `--unsafe`, allowlist par défaut (§4.1).
4. 5 assign-in-if (§5.3).
5. Décision nommage/doc `schedule` (§5.2).

### M1 · Cœur fonctionnel (gate beta-shippable)
6. Filet couverture MariaDB + `internal/export` (§3.5).
7. Phase 4 : fix bug d'ID (§3.1), schema `idempotency_key` (§3.2), Claimer réel memory+MariaDB, Recoverer réel (§3.3), conso `on_recovery` (§3.4), réécriture commentaires export (§4.2).
8. Tests e2e idempotency + recovery (§3.5).

### M2 · Finitions pour trancher
9. Trancher drift UI + resync docs (§5.1).
10. Fix goto reset SSE (§5.2).
11. Découpe `engine.go` (filet tests) + décision `server.go` (§5.3).
12. Docs SaaS alignées (§4.3).

## 7. Hors périmètre v1 (post-v1, avec le SaaS)

- ClickHouse comme **vrai** sink analytics alimenté depuis MariaDB.
- Multi-réplicas / HA : ownership recovery via `GET_LOCK` (section critique brève, conn épinglée) ou CAS de statut ; Locker MariaDB.
- Client export SaaS distant (impls remotes des interfaces `export.*`).
- Durcissement compile-time via build `-tags saas`.

## 8. Risques & garde-fous

- **Faux sentiment de complétude** (risque historique) : aucune fonctionnalité exposée ne doit rester noop fail-open. Les noop Claimer/Recoverer disparaissent ou ne sont câblés que pour le backend memory (où ils sont *réels*, pas fail-open).
- **Découpe `engine.go`** : cœur du moteur — ne la faire qu'avec la couverture existante verte comme garde-fou.
- **Régression sécu** : la suppression de `SelfHosted` ne doit pas ouvrir `exec`/`js`/`file.*` par défaut — l'allowlist reste active sans `--unsafe`.
