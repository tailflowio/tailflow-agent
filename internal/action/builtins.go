package action

// unsafeRegistration pairs a name with its factory.
type unsafeRegistration struct {
	name    string
	factory ActionFactory
}

// unsafeRegistrations is populated by init() in builtins_unsafe.go (excluded in SaaS builds).
var unsafeRegistrations []unsafeRegistration

// RegisterBuiltins registers all built-in actions.
// Actions guarded by the "saas" build tag (exec, js, file.*) are only
// included when building without -tags saas.
func RegisterBuiltins(reg *Registry) {
	// Safe actions — always available
	reg.Register("set", NewSetAction)
	reg.Register("log", NewLogAction)
	reg.Register("http", NewHTTPAction)
	reg.Register("condition", NewConditionAction)
	reg.Register("loop", NewLoopAction)
	reg.Register("json.decode", NewJSONDecodeAction)
	reg.Register("json.encode", NewJSONEncodeAction)
	reg.Register("template", NewTemplateAction)
	reg.Register("delay", NewDelayAction)
	reg.Register("response", NewResponseAction)
	reg.Register("wait.webhook", NewWaitWebhookAction)
	reg.Register("wait.rabbitmq", NewWaitRabbitMQAction)
	reg.Register("rabbitmq.shovel", NewRabbitMQShovelAction)
	reg.Register("validate", NewValidateAction)
	reg.Register("lock", NewLockAction)
	reg.Register("unlock", NewUnlockAction)
	reg.Register("sql.query", NewSQLQueryAction)
	reg.Register("sql.exec", NewSQLExecAction)
	reg.Register("sql.begin", NewSQLBeginAction)
	reg.Register("sql.commit", NewSQLCommitAction)
	reg.Register("sql.rollback", NewSQLRollbackAction)
	reg.Register("schedule", NewScheduleAction)
	reg.Register("array.sort", NewArraySortAction)
	reg.Register("array.filter", NewArrayFilterAction)
	reg.Register("array.map", NewArrayMapAction)
	reg.Register("array.uniq", NewArrayUniqAction)
	reg.Register("array.pick", NewArrayPickAction)
	reg.Register("array.concat", NewArrayConcatAction)
	reg.Register("object", NewObjectAction)
	reg.Register("math", NewMathAction)
	reg.Register("hash", NewHashAction)
	reg.Register("string.replace", NewStringReplaceAction)
	reg.Register("string.match_all", NewStringMatchAllAction)
	reg.Register("kv.get", NewKVGetAction)
	reg.Register("kv.set", NewKVSetAction)
	reg.Register("kv.delete", NewKVDeleteAction)

	// Unsafe actions — excluded from SaaS builds
	for _, r := range unsafeRegistrations {
		reg.Register(r.name, r.factory)
	}
}
