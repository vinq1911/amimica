package app

import (
	"fmt"
	"os"

	"github.com/vinq1911/amimica/internal/config"
)

// loadConfig loads configuration from a file path and applies environment
// overrides. Callers should apply flag overrides then call config.Validate().
func loadConfig(configPath string) (*config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}
	config.ApplyEnv(cfg)
	return cfg, nil
}

// exitError prints an error and returns the given exit code.
func exitError(err error, code int) int {
	fmt.Fprintf(os.Stderr, "amimica: %v\n", err)
	return code
}
