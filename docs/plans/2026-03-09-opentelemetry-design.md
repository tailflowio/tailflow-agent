# OpenTelemetry Integration Design — TailFlow Agent

## Principe directeur

**Zero overhead quand désactivé.** Aucune dépendance OTel n'est importée dans le hot path si aucun endpoint n'est configuré. Le provider retourne un `noop.TracerProvider` / `noop.MeterProvider` / `noop.LoggerProvider` — tous les appels sont des no-ops avec zéro allocation.

## Les 3 piliers OTel

### 1. Tracing

Modèle de traces :

```
Trace: workflow execution
├── Span: "workflow {name}" (root)
│   ├── attr: workflow.name, workflow.trigger, workflow.id
│   ├── Span: "step {name}"
│   │   ├── attr: step.name, step.action, step.status
│   │   ├── attr: step.input, step.output (masqués via sensitive fields existants)
│   │   ├── Span: "retry 1" (si retry)
│   │   │   └── event: error details
│   │   └── Span: "retry 2"
│   │       └── status: OK
│   ├── Span: "step {name2}" (parallèle)
│   └── Span: "step {name3}"
```

- Un span par retry (chaque tentative = span enfant du step)
- Propagation W3C Trace Context (`traceparent`/`tracestate`) injectée automatiquement dans les appels HTTP sortants de l'action `http`
- Données sensibles masquées via le système `sensitive` existant

### 2. Métriques

Instruments OTel qui lisent les snapshots du collector existant (`internal/metrics/`). Le collector custom reste intact pour la UI et le SaaS exporter.

| Métrique | Type | Source |
|---|---|---|
| `tailflow.cpu.usage` | Gauge | Collector existant |
| `tailflow.memory.rss` | Gauge | Collector existant |
| `tailflow.goroutines` | Gauge | Collector existant |
| `tailflow.heap.alloc` | Gauge | Collector existant |
| `tailflow.network.rx_bytes` | Counter | Collector existant |
| `tailflow.network.tx_bytes` | Counter | Collector existant |
| `tailflow.uptime` | Gauge | Collector existant |

### 3. Logs

Bridge slog → OTel Logs via `otelslog`. Les logs sont automatiquement corrélés aux traces (`trace_id`/`span_id`). Quand désactivé, slog fonctionne exactement comme aujourd'hui.

## Configuration

| Flag CLI | Env var OTel | Défaut | Description |
|---|---|---|---|
| `--otel-endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | *(vide = désactivé)* | Endpoint OTLP/HTTP |
| `--otel-service-name` | `OTEL_SERVICE_NAME` | `tailflow` | Nom du service |

- Les env vars suffisent seules à activer OTel
- Les flags CLI prennent priorité sur les env vars
- Protocole : OTLP/HTTP uniquement (pas de gRPC)

## Commandes supportées

`serve` et `run`.

## Architecture interne

```
internal/otel/
├── provider.go      — Init/shutdown TracerProvider + MeterProvider + LoggerProvider, noop si pas configuré
├── tracing.go       — Helpers spans workflow/step/retry
├── metrics.go       — Instruments OTel qui lisent le metrics collector
├── logging.go       — Bridge slog → OTel Logs
└── propagation.go   — Injection W3C dans les requêtes HTTP sortantes
```

Point d'intégration clé : `engine.go` crée les spans au moment de l'exécution des steps. Le provider est initialisé au démarrage dans `server.go` / `main.go` et passé via `context.Context`.

## Ce qu'on ne fait PAS

- Pas de gRPC (OTLP/HTTP uniquement)
- Pas de tracing sur la commande `test`
- Pas de flag `--otel-redact-data` séparé (réutilise `sensitive`)
- Pas de remplacement du collector custom ni du SaaS exporter
