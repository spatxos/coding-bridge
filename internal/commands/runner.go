// Package commands 提供安全的命令执行功能。
// 所有命令执行前必须经过白名单/黑名单检查。
package commands

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner 命令执行器
type Runner struct {
	projectRoot     string
	allowedCommands []string
	forbiddenCmds   []string
	timeout         time.Duration
}

// NewRunner 创建命令执行器
func NewRunner(projectRoot string, allowedCmds, forbiddenCmds []string, timeout time.Duration) *Runner {
	return &Runner{
		projectRoot:     projectRoot,
		allowedCommands: allowedCmds,
		forbiddenCmds:   forbiddenCmds,
		timeout:         timeout,
	}
}

// IsAllowed 检查命令是否在白名单中
func (r *Runner) IsAllowed(cmd string) bool {
	// 检查黑名单
	for _, forbidden := range r.forbiddenCmds {
		if strings.Contains(strings.ToLower(cmd), strings.ToLower(forbidden)) {
			return false
		}
	}

	// 如果有白名单，检查是否匹配
	if len(r.allowedCommands) > 0 {
		for _, allowed := range r.allowedCommands {
			if commandMatches(cmd, allowed) {
				return true
			}
		}
		return false
	}

	// 没有白名单时，只要不在黑名单中就允许
	return true
}

func commandMatches(command, allowed string) bool {
	command = strings.TrimSpace(command)
	allowed = strings.TrimSpace(allowed)
	if command == allowed {
		return true
	}
	return strings.HasPrefix(command, allowed+" ") || strings.HasPrefix(command, allowed+"\t")
}

// CommandResult 命令执行结果
type CommandResult struct {
	Command  string `json:"command"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
	TimedOut bool   `json:"timed_out"`
}

// BuildTestResult 构建/测试结果
type BuildTestResult struct {
	Success  bool   `json:"success"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

// Run 执行命令（同步，带超时）
func (r *Runner) Run(ctx context.Context, cmdStr string) (*CommandResult, error) {
	// 安全检查
	if !r.IsAllowed(cmdStr) {
		return nil, fmt.Errorf("command %q is not allowed", cmdStr)
	}

	startTime := time.Now()

	// 创建带超时的 context
	cmdCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// 解析命令
	cmd := r.parseCommand(cmdCtx, cmdStr)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(startTime)

	result := &CommandResult{
		Command:  cmdStr,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration.Round(time.Millisecond).String(),
	}

	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
			return result, fmt.Errorf("command timed out after %s", r.timeout)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, nil // 不返回错误，让调用方根据 ExitCode 判断
	}

	result.ExitCode = 0
	return result, nil
}

// parseCommand 解析并创建命令
func (r *Runner) parseCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	// 简单按空格分割，支持引号包裹的参数
	parts := splitCommand(cmdStr)
	if len(parts) == 0 {
		return exec.CommandContext(ctx, "")
	}

	if len(parts) == 1 {
		cmd := exec.CommandContext(ctx, parts[0])
		cmd.Dir = r.projectRoot
		return cmd
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = r.projectRoot
	return cmd
}

// splitCommand 分割命令行（保留引号内的空格）
func splitCommand(cmd string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '"' || c == '\'':
			if inQuote && c == quoteChar {
				inQuote = false
			} else if !inQuote {
				inQuote = true
				quoteChar = c
			} else {
				current.WriteByte(c)
			}
		case c == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// RunBuild 执行构建命令
func (r *Runner) RunBuild(ctx context.Context) (*BuildTestResult, error) {
	// 尝试常见构建命令
	buildCmds := []string{
		"go build ./...",
		"dotnet build",
		"npm run build",
	}

	for _, cmd := range buildCmds {
		if !r.IsAllowed(cmd) {
			continue
		}
		result, err := r.Run(ctx, cmd)
		if err != nil {
			// 超时不退出
			continue
		}
		if result.ExitCode == 0 {
			return &BuildTestResult{
				Success:  true,
				Output:   result.Stdout,
				ExitCode: result.ExitCode,
			}, nil
		}
	}

	return &BuildTestResult{
		Success:  false,
		Output:   "no successful build command found",
		ExitCode: -1,
	}, nil
}

// RunTest 执行测试命令
func (r *Runner) RunTest(ctx context.Context) (*BuildTestResult, error) {
	testCmds := []string{
		"go test ./...",
		"dotnet test",
		"npm test",
		"pytest",
	}

	for _, cmd := range testCmds {
		if !r.IsAllowed(cmd) {
			continue
		}
		result, err := r.Run(ctx, cmd)
		if err != nil {
			continue
		}

		output := result.Stdout
		if result.Stderr != "" {
			output += "\n" + result.Stderr
		}

		return &BuildTestResult{
			Success:  result.ExitCode == 0,
			Output:   output,
			ExitCode: result.ExitCode,
		}, nil
	}

	return &BuildTestResult{
		Success:  false,
		Output:   "no test command found",
		ExitCode: -1,
	}, nil
}
