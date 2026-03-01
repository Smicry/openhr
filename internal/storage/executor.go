package storage

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/openhr/pkg/utils/logger"
)

// Executor - Command executor
type Executor struct{}

// NewExecutor - Creates executor
func NewExecutor() *Executor {
	return &Executor{}
}

// Run - Execute command
func (e *Executor) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	logger.Debug("Executing command: %s %s", name, strings.Join(args, " "))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	if err != nil {
		if stderr.Len() > 0 {
			return output, fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(stderr.String()))
		}
		return output, err
	}

	return output, nil
}

// CheckCommandExists - Check if command exists
func (e *Executor) CheckCommandExists(name string) bool {
	cmd := exec.Command("which", name)
	err := cmd.Run()
	return err == nil
}

// GetCommandPath - Get command path
func (e *Executor) GetCommandPath(name string) string {
	output, err := e.Run("which", name)
	if err == nil {
		return strings.TrimSpace(output)
	}
	return ""
}
