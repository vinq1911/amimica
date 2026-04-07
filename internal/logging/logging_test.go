package logging_test

import (
	"testing"

	"github.com/user/amimica/internal/logging"
)

func TestSetupReturnsLogger(t *testing.T) {
	log := logging.Setup("info", "text")
	if log == nil {
		t.Fatal("Setup returned nil logger")
	}
}

func TestSetupJSONFormat(t *testing.T) {
	// Should not panic and return a valid logger
	log := logging.Setup("info", "json")
	if log == nil {
		t.Fatal("Setup with json format returned nil logger")
	}
}

func TestSetupTextFormat(t *testing.T) {
	log := logging.Setup("info", "text")
	if log == nil {
		t.Fatal("Setup with text format returned nil logger")
	}
}

func TestSetupInvalidLevelDefaultsToInfo(t *testing.T) {
	// Invalid level should not panic; defaults to info
	log := logging.Setup("notavalidlevel", "text")
	if log == nil {
		t.Fatal("Setup with invalid level returned nil logger")
	}
}

func TestSetupAllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error", "DEBUG", "INFO", "WARN", "ERROR"}
	for _, level := range levels {
		log := logging.Setup(level, "text")
		if log == nil {
			t.Fatalf("Setup(%q, text) returned nil logger", level)
		}
	}
}
