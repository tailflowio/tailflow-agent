package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type EnrichTestSuite struct {
	suite.Suite
	exporter *tracetest.InMemoryExporter
	provider *sdktrace.TracerProvider
}

func TestEnrichStepSpan(t *testing.T) {
	suite.Run(t, new(EnrichTestSuite))
}

func (s *EnrichTestSuite) SetupTest() {
	s.exporter = tracetest.NewInMemoryExporter()
	s.provider = sdktrace.NewTracerProvider(sdktrace.WithSyncer(s.exporter))
}

func (s *EnrichTestSuite) TearDownTest() {
	_ = s.provider.Shutdown(context.Background())
}

func (s *EnrichTestSuite) startAndEnrich(
	action string, config map[string]any, output any,
) map[string]any {
	ctx, span := s.provider.Tracer("test").Start(context.Background(), "test-span")
	_ = ctx
	EnrichStepSpan(span, action, config, output)
	span.End()

	spans := s.exporter.GetSpans()
	s.Require().Len(spans, 1)

	return spanAttrMap(spans[0])
}

func (s *EnrichTestSuite) TestHTTP() {
	attrs := s.startAndEnrich("http", map[string]any{
		"method": "post",
		"url":    "https://example.com/api",
	}, map[string]any{
		"status": 201,
	})

	s.Equal("POST", attrs["http.request.method"])
	s.Equal("https://example.com/api", attrs["url.full"])
	s.Equal(int64(201), attrs["http.response.status_code"])
}

func (s *EnrichTestSuite) TestHTTPNilOutput() {
	attrs := s.startAndEnrich("http", map[string]any{
		"method": "get",
		"url":    "https://example.com",
	}, nil)

	s.Equal("GET", attrs["http.request.method"])
	s.Equal("https://example.com", attrs["url.full"])
	s.NotContains(attrs, "http.response.status_code")
}

func (s *EnrichTestSuite) TestHTTPNonMapOutput() {
	attrs := s.startAndEnrich("http", map[string]any{
		"method": "get",
	}, "not-a-map")

	s.Equal("GET", attrs["http.request.method"])
	s.NotContains(attrs, "http.response.status_code")
}

func (s *EnrichTestSuite) TestSQLQuery() {
	attrs := s.startAndEnrich("sql.query", map[string]any{
		"dsn":   "postgres://localhost/db",
		"query": "SELECT * FROM users",
	}, nil)

	s.Equal("postgresql", attrs["db.system"])
	s.Equal("SELECT * FROM users", attrs["db.statement"])
	s.Equal("SELECT", attrs["db.operation"])
}

func (s *EnrichTestSuite) TestSQLExec() {
	attrs := s.startAndEnrich("sql.exec", map[string]any{
		"dsn":   "mysql://localhost/db",
		"query": "INSERT INTO users (name) VALUES ('test')",
	}, nil)

	s.Equal("mysql", attrs["db.system"])
	s.Equal("INSERT INTO users (name) VALUES ('test')", attrs["db.statement"])
	s.Equal("INSERT", attrs["db.operation"])
}

func (s *EnrichTestSuite) TestSQLTransaction() {
	for _, tc := range []struct {
		action    string
		operation string
	}{
		{"sql.begin", "BEGIN"},
		{"sql.commit", "COMMIT"},
		{"sql.rollback", "ROLLBACK"},
	} {
		s.exporter.Reset()

		attrs := s.startAndEnrich(tc.action, map[string]any{
			"dsn": "postgres://localhost/db",
		}, nil)

		s.Equal("postgresql", attrs["db.system"], "action: %s", tc.action)
		s.Equal(tc.operation, attrs["db.operation"], "action: %s", tc.action)
	}
}

func (s *EnrichTestSuite) TestKV() {
	for _, tc := range []struct {
		action    string
		operation string
	}{
		{"kv.get", "GET"},
		{"kv.set", "SET"},
		{"kv.delete", "DELETE"},
	} {
		s.exporter.Reset()

		attrs := s.startAndEnrich(tc.action, map[string]any{
			"key": "my-key",
		}, nil)

		s.Equal("redis", attrs["db.system"], "action: %s", tc.action)
		s.Equal(tc.operation, attrs["db.operation"], "action: %s", tc.action)
		s.Equal("my-key", attrs["db.redis.key"], "action: %s", tc.action)
	}
}

func (s *EnrichTestSuite) TestRabbitMQ() {
	attrs := s.startAndEnrich("rabbitmq.shovel", map[string]any{
		"queue": "orders",
	}, nil)

	s.Equal("rabbitmq", attrs["messaging.system"])
	s.Equal("orders", attrs["messaging.destination.name"])
}

func (s *EnrichTestSuite) TestWaitWebhook() {
	attrs := s.startAndEnrich("wait.webhook", map[string]any{
		"path": "/callback",
	}, nil)

	s.Equal("webhook", attrs["tailflow.wait.type"])
	s.Equal("/callback", attrs["tailflow.wait.path"])
}

func (s *EnrichTestSuite) TestWaitRabbitMQ() {
	attrs := s.startAndEnrich("wait.rabbitmq", map[string]any{
		"queue": "events",
	}, nil)

	s.Equal("rabbitmq", attrs["tailflow.wait.type"])
	s.Equal("events", attrs["messaging.destination.name"])
}

func (s *EnrichTestSuite) TestLock() {
	attrs := s.startAndEnrich("lock", map[string]any{
		"key": "resource-1",
	}, nil)

	s.Equal("resource-1", attrs["tailflow.lock.key"])
}

func (s *EnrichTestSuite) TestExec() {
	attrs := s.startAndEnrich("exec", map[string]any{
		"command": "echo hello",
	}, nil)

	s.Equal("echo hello", attrs["process.command"])
}

func (s *EnrichTestSuite) TestFileReadWrite() {
	for _, action := range []string{"file.read", "file.write"} {
		s.exporter.Reset()

		attrs := s.startAndEnrich(action, map[string]any{
			"path": "/tmp/data.txt",
		}, nil)

		s.Equal("/tmp/data.txt", attrs["tailflow.file.path"], "action: %s", action)
	}
}

func (s *EnrichTestSuite) TestUnknownAction() {
	attrs := s.startAndEnrich("set", map[string]any{
		"key": "val",
	}, nil)

	s.Empty(attrs)
}

func (s *EnrichTestSuite) TestConfigStringNonString() {
	val, ok := configString(map[string]any{"key": 123}, "key")

	s.False(ok)
	s.Empty(val)
}

func (s *EnrichTestSuite) TestConfigStringMissing() {
	val, ok := configString(map[string]any{}, "key")

	s.False(ok)
	s.Empty(val)
}

func (s *EnrichTestSuite) TestToInt64AllBranches() {
	s.Equal(int64(42), toInt64(42))
	s.Equal(int64(99), toInt64(int64(99)))
	s.Equal(int64(3), toInt64(float64(3.7)))
	s.Equal(int64(0), toInt64("not-a-number"))
}

func (s *EnrichTestSuite) TestDbSystemFromDSNUnknown() {
	s.Equal("other", dbSystemFromDSN("sqlite:///test.db"))
}

func (s *EnrichTestSuite) TestDbSystemFromDSNMySQL() {
	s.Equal("mysql", dbSystemFromDSN("root:pass@tcp(localhost:3306)/mydb"))
}

func (s *EnrichTestSuite) TestDbNameFromDSN() {
	s.Equal("mydb", dbNameFromDSN("root:pass@tcp(localhost:3306)/mydb"))
	s.Equal("mydb", dbNameFromDSN("root:pass@tcp(localhost:3306)/mydb?charset=utf8"))
	s.Equal("", dbNameFromDSN("no-slash-here"))
}

func (s *EnrichTestSuite) TestSqlOperationEmpty() {
	s.Equal("", sqlOperation(""))
	s.Equal("", sqlOperation("   "))
}

func (s *EnrichTestSuite) TestUnlock() {
	attrs := s.startAndEnrich("unlock", map[string]any{
		"key": "resource-2",
	}, nil)

	s.Equal("resource-2", attrs["tailflow.lock.key"])
}
