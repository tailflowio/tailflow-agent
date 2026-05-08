package main

import (
	"os"
	"strings"
)

func detectNoColor(flagValue bool) bool {
	if flagValue {
		return true
	}

	_, ok := os.LookupEnv("NO_COLOR")
	if ok {
		return true
	}

	fi, err := os.Stdout.Stat()
	if err != nil {
		return true
	}

	if fi.Mode()&os.ModeCharDevice == 0 {
		return true // piped / not a TTY
	}

	return false
}

func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

func flagOrEnv(flagVal, envName string) string {
	if flagVal != "" {
		return flagVal
	}

	return os.Getenv(envName)
}

func parseParams(raw []string) map[string]any {
	params := make(map[string]any)

	for _, p := range raw {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) == 2 {
			params[parts[0]] = parts[1]
		}
	}

	return params
}

func cliAllowedActions(all []string) []string {
	allowed := make([]string, 0, len(all))

	for _, name := range all {
		if strings.HasPrefix(name, "wait.") ||
			name == "schedule" {
			continue
		}

		allowed = append(allowed, name)
	}

	return allowed
}
