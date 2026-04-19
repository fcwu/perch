package main

import (
	"fmt"
	"os"
)

// OperatingMode selects single-user or multi-user behaviour.
type OperatingMode string

const (
	ModeSingle OperatingMode = "single"
	ModeMulti  OperatingMode = "multi"
)

// parseOperatingMode reads PERCH_MODE and validates it.
func parseOperatingMode() (OperatingMode, error) {
	v := os.Getenv("PERCH_MODE")
	if v == "" {
		return ModeSingle, nil
	}
	switch OperatingMode(v) {
	case ModeSingle:
		return ModeSingle, nil
	case ModeMulti:
		return ModeMulti, nil
	default:
		return "", fmt.Errorf("invalid PERCH_MODE %q: must be \"single\" or \"multi\"", v)
	}
}

// validateMultiModeEnv returns an error if PERCH_MODE=multi but GitLab OAuth
// env vars are missing.
func validateMultiModeEnv() error {
	missing := []string{}
	for _, k := range []string{"GITLAB_CLIENT_ID", "GITLAB_CLIENT_SECRET", "GITLAB_URL"} {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("PERCH_MODE=multi requires GitLab OAuth configuration; missing: %v", missing)
	}
	return nil
}
