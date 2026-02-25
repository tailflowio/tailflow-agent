//go:build !saas

package action

// registerUnsafeBuiltins registers actions that allow arbitrary code execution
// or filesystem access. Excluded from SaaS builds via the "saas" build tag.
func init() {
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"exec", NewExecAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"js", NewJSAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"file.read", NewFileReadAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"file.write", NewFileWriteAction})
}
