package main

import (
	"os"

	"github.com/openhr/cmd"
	"github.com/openhr/internal/config"
	"github.com/openhr/pkg/utils/logger"
)

func main() {
	// Initialize config
	config.Init()

	// Initialize logger
	logger.Init()

	// Check root privileges
	if os.Geteuid() != 0 {
		logger.Warn("Running as non-root user, some operations may require root privileges")
	}

	// Execute command
	if err := cmd.Execute(); err != nil {
		logger.Error("Execution failed: %v", err)
		os.Exit(1)
	}
}
