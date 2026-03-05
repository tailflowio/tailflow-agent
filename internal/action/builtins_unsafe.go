//go:build !saas

package action

func init() { //nolint:gochecknoinits
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"exec", NewExecAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"js", NewJSAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"file.read", NewFileReadAction})
	unsafeRegistrations = append(unsafeRegistrations, unsafeRegistration{"file.write", NewFileWriteAction})
}
