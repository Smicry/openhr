package storage

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/openhr/pkg/utils/logger"
)

// Executor 命令执行器
type Executor struct {
	timeout time.Duration
}

// NewExecutor 创建执行器
func NewExecutor() *Executor {
	return &Executor{
		timeout: 300 * time.Second, // 默认5分钟超时
	}
}

// Run 执行命令
func (e *Executor) Run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	logger.Debug("执行命令: %s %s", name, strings.Join(args, " "))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("命令执行超时")
		}
		if stderr.Len() > 0 {
			return output, fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(stderr.String()))
		}
		return output, err
	}

	return output, nil
}

// RunSimple 执行简单命令（无超时）
func (e *Executor) RunSimple(name string, args ...string) (string, error) {
	return e.Run(name, args...)
}

// RunWithTimeout 指定超时执行命令
func (e *Executor) RunWithTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("命令执行超时")
		}
		if stderr.Len() > 0 {
			return stdout.String(), fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), err
	}

	return stdout.String(), nil
}

// CheckCommandExists 检查命令是否存在
func (e *Executor) CheckCommandExists(name string) bool {
	cmd := exec.Command("which", name)
	err := cmd.Run()
	return err == nil
}

// GetCommandPath 获取命令路径
func (e *Executor) GetCommandPath(name string) string {
	output, err := e.Run("which", name)
	if err == nil {
		return strings.TrimSpace(output)
	}
	return ""
}
