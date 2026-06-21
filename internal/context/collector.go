// Package context 提供任务上下文的收集、过滤与脱敏功能。
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultMaxFileBytes  = 100 * 1024
	defaultMaxTotalBytes = 64 * 1024

	// ErrForbiddenInternalContext 错误码：禁止将内部状态文件发送到 Executor 上下文
	ErrForbiddenInternalContext = "FORBIDDEN_INTERNAL_CONTEXT"
)

// InternalStateDenyPatterns 内部状态目录模式，绝对禁止进入 Executor 上下文
var InternalStateDenyPatterns = []string{
	".coding-bridge",
	".coding-bridge/*",
	".coding-bridge/**",
	".git",
	".git/*",
	".git/**",
	"node_modules",
	"node_modules/*",
	"node_modules/**",
	"bin",
	"bin/*",
	"bin/**",
	"obj",
	"obj/*",
	"obj/**",
	"dist",
	"dist/*",
	"dist/**",
	"build",
	"build/*",
	"build/**",
}

// IsInternalStatePath 检查路径是否命中内部状态禁止模式
func IsInternalStatePath(relPath string) bool {
	clean := filepath.ToSlash(strings.TrimSpace(relPath))
	if clean == "" {
		return false
	}
	// 去除前面的 ./ 如果有
	clean = strings.TrimPrefix(clean, "./")

	for _, deny := range InternalStateDenyPatterns {
		matched, err := filepath.Match(deny, clean)
		if err == nil && matched {
			return true
		}
		// 也检查父目录匹配
		parts := strings.Split(clean, "/")
		prefix := parts[0]
		if prefix == "" && len(parts) > 1 {
			prefix = parts[1]
		}
		matched, err = filepath.Match(deny, prefix)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// Collector 上下文收集器
type Collector struct {
	projectRoot    string
	allowedFiles   []string
	forbiddenFiles []string
	maxFileBytes   int
	maxTotalBytes  int
}

// NewCollector 创建收集器
func NewCollector(projectRoot string, allowedFiles, forbiddenFiles []string) *Collector {
	blocked := append([]string{}, DefaultForbiddenPatterns...)
	blocked = append(blocked, forbiddenFiles...)
	return &Collector{
		projectRoot:    projectRoot,
		allowedFiles:   allowedFiles,
		forbiddenFiles: blocked,
		maxFileBytes:   defaultMaxFileBytes,
		maxTotalBytes:  defaultMaxTotalBytes,
	}
}

// CollectedFile 收集到的文件上下文
type CollectedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
	Skipped bool   `json:"skipped,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// Context 收集到的完整上下文
type Context struct {
	Files        []CollectedFile `json:"files"`
	TotalSize    int             `json:"total_size"`
	TotalFiles   int             `json:"total_files"`
	SkippedFiles int             `json:"skipped_files"`
}

// Collect 收集允许文件的上下文
func (c *Collector) Collect() (*Context, error) {
	// 首先检查 allowed_files 是否包含内部状态路径
	for _, pattern := range c.allowedFiles {
		rel := filepath.ToSlash(pattern)
		if IsInternalStatePath(rel) {
			return nil, fmt.Errorf("%s: .coding-bridge internal state files cannot be sent to Executor context: %s", ErrForbiddenInternalContext, pattern)
		}
	}

	result := &Context{}
	seen := make(map[string]bool)

	for _, pattern := range c.allowedFiles {
		// 展开 glob 模式
		matches, err := filepath.Glob(filepath.Join(c.projectRoot, pattern))
		if err != nil {
			return nil, fmt.Errorf("expand allowed file pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			result.SkippedFiles++
			result.Files = append(result.Files, CollectedFile{
				Path:    filepath.ToSlash(pattern),
				Skipped: true,
				Reason:  "file not found",
			})
		}

		for _, match := range matches {
			relPath, err := filepath.Rel(c.projectRoot, match)
			if err != nil {
				relPath = match
			}
			relPath = filepath.ToSlash(relPath)

			// 二次检查：glob 展开后的实际路径是否命中内部状态目录
			if IsInternalStatePath(relPath) {
				return nil, fmt.Errorf(
					"%s: internal state path expanded from allowed_files: %s",
					ErrForbiddenInternalContext,
					relPath,
				)
			}

			if seen[relPath] {
				continue
			}
			seen[relPath] = true

			// 检查是否是禁止文件
			if c.isForbidden(match) {
				result.SkippedFiles++
				result.Files = append(result.Files, CollectedFile{
					Path:    relPath,
					Skipped: true,
					Reason:  "forbidden file",
				})
				continue
			}

			data, err := os.ReadFile(match)
			if err != nil {
				result.Files = append(result.Files, CollectedFile{
					Path:    relPath,
					Skipped: true,
					Reason:  fmt.Sprintf("read error: %v", err),
				})
				result.SkippedFiles++
				continue
			}

			if len(data) > c.maxFileBytes {
				result.Files = append(result.Files, CollectedFile{
					Path:    relPath,
					Skipped: true,
					Reason:  "file too large (>100KB)",
				})
				result.SkippedFiles++
				continue
			}

			content := Sanitize(string(data))
			remaining := c.maxTotalBytes - result.TotalSize
			if remaining <= 0 {
				result.Files = append(result.Files, CollectedFile{
					Path:    relPath,
					Skipped: true,
					Reason:  "context budget exhausted",
				})
				result.SkippedFiles++
				continue
			}
			if len(content) > remaining {
				const marker = "\n/* context truncated by coding-bridge */"
				if remaining > len(marker) {
					content = content[:remaining-len(marker)] + marker
				} else {
					content = content[:remaining]
				}
			}

			cf := CollectedFile{
				Path:    relPath,
				Content: content,
				Size:    len(content),
			}
			result.Files = append(result.Files, cf)
			result.TotalSize += len(content)
			result.TotalFiles++
		}
	}

	return result, nil
}

// isForbidden 检查文件是否在禁止列表中
func (c *Collector) isForbidden(path string) bool {
	relPath, err := filepath.Rel(c.projectRoot, path)
	if err != nil {
		relPath = filepath.Base(path)
	}

	for _, pattern := range c.forbiddenFiles {
		matched, err := filepath.Match(pattern, relPath)
		if err == nil && matched {
			return true
		}
		// 也检查文件名
		matched, err = filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// DefaultForbiddenPatterns 默认禁止的文件模式
var DefaultForbiddenPatterns = []string{
	".env",
	".env.*",
	"secrets.json",
	"secrets.yaml",
	"appsettings.Production.json",
	"*.pfx",
	"*.p12",
	"*.key",
	"*.pem",
	"*.cer",
	"id_rsa",
	"id_ed25519",
	"*.token",
	"credentials.json",
	"*.secret",
}

// IsDefaultForbidden 检查是否为默认禁止文件
func IsDefaultForbidden(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range DefaultForbiddenPatterns {
		matched, err := filepath.Match(pattern, base)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// Sanitize 脱敏内容中的敏感信息
func Sanitize(content string) string {
	// 替换常见的敏感模式
	replacements := map[string]string{
		// API Key 模式
		`(?i)(api[_-]?key|apikey|api[_-]?secret)\s*[:=]\s*["']?[^"'\s]+["']?`: "[REDACTED_API_KEY]",
	}

	for pattern, replacement := range replacements {
		// 简单字符串包含检查（不需要正则）
		lowerContent := strings.ToLower(content)
		for _, kw := range []string{"api_key", "apikey", "api_secret", "apisecret"} {
			if strings.Contains(lowerContent, kw) {
				content = maskLine(content, kw)
			}
		}
		_ = pattern
		_ = replacement
	}

	return content
}

// maskLine 脱敏包含特定关键词的行
func maskLine(content, keyword string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), keyword) {
			// 替换值部分
			if idx := strings.IndexAny(line, "=:"); idx >= 0 {
				lines[i] = line[:idx+1] + " [REDACTED]"
			} else {
				lines[i] = "[REDACTED]"
			}
		}
	}
	return strings.Join(lines, "\n")
}
