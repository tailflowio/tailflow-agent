package otel

import (
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func EnrichStepSpan(span trace.Span, actionName string, config map[string]any, output any) {
	switch actionName {
	case "http":
		enrichHTTP(span, config, output)
	case "sql.query", "sql.exec":
		enrichSQL(span, config)
	case "sql.begin":
		enrichSQLTx(span, config, "BEGIN")
	case "sql.commit":
		enrichSQLTx(span, config, "COMMIT")
	case "sql.rollback":
		enrichSQLTx(span, config, "ROLLBACK")
	case "kv.get":
		enrichKV(span, config, "GET")
	case "kv.set":
		enrichKV(span, config, "SET")
	case "kv.delete":
		enrichKV(span, config, "DELETE")
	case "rabbitmq.shovel":
		enrichRabbitMQ(span, config)
	case "wait.webhook":
		enrichWaitWebhook(span, config)
	case "wait.rabbitmq":
		enrichWaitRabbitMQ(span, config)
	case "lock", "unlock":
		enrichLock(span, config)
	case "exec":
		enrichExec(span, config)
	case "file.read", "file.write":
		enrichFile(span, config)
	}
}

func enrichHTTP(span trace.Span, config map[string]any, output any) {
	method, ok := configString(config, "method")
	if ok {
		span.SetAttributes(attribute.String("http.request.method", strings.ToUpper(method)))
	}

	url, ok := configString(config, "url")
	if ok {
		span.SetAttributes(attribute.String("url.full", url))
	}

	outMap, ok := output.(map[string]any)
	if !ok {
		return
	}

	status, ok := outMap["status"]
	if ok {
		span.SetAttributes(attribute.Int64("http.response.status_code", toInt64(status)))
	}
}

func enrichSQL(span trace.Span, config map[string]any) {
	dsn, ok := configString(config, "dsn")
	if ok {
		span.SetAttributes(attribute.String("db.system", dbSystemFromDSN(dsn)))

		name := dbNameFromDSN(dsn)
		if name != "" {
			span.SetAttributes(attribute.String("db.name", name))
		}
	}

	query, ok := configString(config, "query")
	if ok {
		span.SetAttributes(attribute.String("db.statement", query))
		span.SetAttributes(attribute.String("db.operation", sqlOperation(query)))
	}
}

func enrichSQLTx(span trace.Span, config map[string]any, operation string) {
	dsn, ok := configString(config, "dsn")
	if ok {
		span.SetAttributes(attribute.String("db.system", dbSystemFromDSN(dsn)))
	}

	span.SetAttributes(attribute.String("db.operation", operation))
}

func enrichKV(span trace.Span, config map[string]any, operation string) {
	span.SetAttributes(
		attribute.String("db.system", "redis"),
		attribute.String("db.operation", operation),
	)

	key, ok := configString(config, "key")
	if ok {
		span.SetAttributes(attribute.String("db.redis.key", key))
	}
}

func enrichRabbitMQ(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("messaging.system", "rabbitmq"))

	queue, ok := configString(config, "queue")
	if ok {
		span.SetAttributes(attribute.String("messaging.destination.name", queue))
	}
}

func enrichWaitWebhook(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("tailflow.wait.type", "webhook"))

	path, ok := configString(config, "path")
	if ok {
		span.SetAttributes(attribute.String("tailflow.wait.path", path))
	}
}

func enrichWaitRabbitMQ(span trace.Span, config map[string]any) {
	span.SetAttributes(attribute.String("tailflow.wait.type", "rabbitmq"))

	queue, ok := configString(config, "queue")
	if ok {
		span.SetAttributes(attribute.String("messaging.destination.name", queue))
	}
}

func enrichLock(span trace.Span, config map[string]any) {
	key, ok := configString(config, "key")
	if ok {
		span.SetAttributes(attribute.String("tailflow.lock.key", key))
	}
}

func enrichExec(span trace.Span, config map[string]any) {
	command, ok := configString(config, "command")
	if ok {
		span.SetAttributes(attribute.String("process.command", command))
	}
}

func enrichFile(span trace.Span, config map[string]any) {
	path, ok := configString(config, "path")
	if ok {
		span.SetAttributes(attribute.String("tailflow.file.path", path))
	}
}

func configString(config map[string]any, key string) (string, bool) {
	val, ok := config[key]
	if !ok {
		return "", false
	}

	str, ok := val.(string)

	return str, ok
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func dbSystemFromDSN(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres"):
		return "postgresql"
	case strings.HasPrefix(dsn, "mysql"):
		return "mysql"
	case strings.Contains(dsn, "@tcp("):
		return "mysql"
	default:
		return "other"
	}
}

func dbNameFromDSN(dsn string) string {
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return ""
	}

	name := dsn[idx+1:]

	qIdx := strings.Index(name, "?")
	if qIdx >= 0 {
		name = name[:qIdx]
	}

	return name
}

func sqlOperation(query string) string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return ""
	}

	return strings.ToUpper(strings.Fields(trimmed)[0])
}
