// Package patch 提供 unified diff 的解析、校验与安全应用功能。
// 执行模型只能返回 unified diff，所有 diff 必须经过校验才能应用。
package patch

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DiffFile 表示一个文件的 diff 信息
type DiffFile struct {
	OrigPath string // 原始文件路径
	NewPath  string // 新文件路径（重命名时）
	Hunks    []Hunk // 修改块
	IsNew    bool   // 是否是新文件
	IsDelete bool   // 是否是删除
	IsRename bool   // 是否是重命名
}

// Hunk 表示一个修改块
type Hunk struct {
	OrigStart int      // 原始文件起始行
	OrigLines int      // 原始文件行数
	NewStart  int      // 新文件起始行
	NewLines  int      // 新文件行数
	Lines     []string // 修改行（包含上下文）
}

// ParseResult unified diff 解析结果
type ParseResult struct {
	Files   []DiffFile
	RawDiff string
}

// Parser 解析 unified diff
type Parser struct{}

// NewParser 创建解析器
func NewParser() *Parser {
	return &Parser{}
}

// 正则表达式
var (
	reDiffHeader = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)
	reOldFile    = regexp.MustCompile(`^--- (?:a/)?(.+)$`)
	reNewFile    = regexp.MustCompile(`^\+\+\+ (?:b/)?(.+)$`)
	reHunkHeader = regexp.MustCompile(`^@@ -(\d+),?(\d*) \+(\d+),?(\d*) @@(.*)$`)
)

// Parse 解析 unified diff 文本
func (p *Parser) Parse(diffText string) (*ParseResult, error) {
	result := &ParseResult{RawDiff: diffText}
	scanner := bufio.NewScanner(strings.NewReader(diffText))
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var currentFile *DiffFile

	for scanner.Scan() {
		line := scanner.Text()

		// 检测 diff header
		if matches := reDiffHeader.FindStringSubmatch(line); matches != nil {
			if currentFile != nil {
				result.Files = append(result.Files, *currentFile)
			}
			currentFile = &DiffFile{
				OrigPath: matches[1],
				NewPath:  matches[2],
			}
			continue
		}

		// 检测 --- a/file
		if matches := reOldFile.FindStringSubmatch(line); matches != nil {
			if currentFile != nil {
				path := strings.TrimPrefix(matches[1], "a/")
				if path == "/dev/null" {
					currentFile.IsNew = true
				} else if currentFile.OrigPath == "" {
					currentFile.OrigPath = path
				}
			}
			continue
		}

		// 检测 +++ b/file
		if matches := reNewFile.FindStringSubmatch(line); matches != nil {
			if currentFile != nil {
				path := strings.TrimPrefix(matches[1], "b/")
				if path == "/dev/null" {
					currentFile.IsDelete = true
				} else if currentFile.NewPath == "" {
					currentFile.NewPath = path
				}
			}
			continue
		}

		// 检测重命名
		if strings.HasPrefix(line, "rename from ") && currentFile != nil {
			currentFile.IsRename = true
			currentFile.OrigPath = strings.TrimPrefix(line, "rename from ")
			continue
		}
		if strings.HasPrefix(line, "rename to ") && currentFile != nil {
			currentFile.NewPath = strings.TrimPrefix(line, "rename to ")
			continue
		}

		// 检测 hunk header
		if matches := reHunkHeader.FindStringSubmatch(line); matches != nil {
			if currentFile == nil {
				return nil, fmt.Errorf("hunk without file header")
			}

			hunk := Hunk{
				OrigStart: parseInt(matches[1]),
				NewStart:  parseInt(matches[3]),
			}

			if matches[2] != "" {
				hunk.OrigLines = parseInt(matches[2])
			} else {
				hunk.OrigLines = 1
			}
			if matches[4] != "" {
				hunk.NewLines = parseInt(matches[4])
			} else {
				hunk.NewLines = 1
			}

			currentFile.Hunks = append(currentFile.Hunks, hunk)
			continue
		}

		// hunk 内容行
		if currentFile != nil && len(currentFile.Hunks) > 0 {
			lastHunkIdx := len(currentFile.Hunks) - 1
			hunk := &currentFile.Hunks[lastHunkIdx]
			hunk.Lines = append(hunk.Lines, line)
		}
	}

	// 最后一个文件
	if currentFile != nil {
		result.Files = append(result.Files, *currentFile)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diff: %w", err)
	}

	if len(result.Files) == 0 {
		return nil, fmt.Errorf("no valid diff found in input")
	}

	return result, nil
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// Validator Patch 校验器
type Validator struct {
	allowedFiles   []string
	forbiddenFiles []string
	requirements   []string
}

// NewValidator 创建校验器
func NewValidator(allowedFiles, forbiddenFiles, requirements []string) *Validator {
	return &Validator{
		allowedFiles:   allowedFiles,
		forbiddenFiles: forbiddenFiles,
		requirements:   requirements,
	}
}

// ValidationError 校验错误
type ValidationError struct {
	File    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// Validate 校验解析后的 diff
func (v *Validator) Validate(result *ParseResult) []error {
	var errs []error

	for _, f := range result.Files {
		for _, path := range []string{f.OrigPath, f.NewPath} {
			if path != "" && path != "/dev/null" && !isSafeRelativePath(path) {
				errs = append(errs, &ValidationError{
					File:    path,
					Message: "absolute paths and paths outside the project are not allowed",
				})
			}
		}

		// 检查是否在 allowed_files 中
		if !v.isAllowed(f.OrigPath) && !v.isAllowed(f.NewPath) {
			errs = append(errs, &ValidationError{
				File:    f.OrigPath,
				Message: fmt.Sprintf("file %q is not in allowed_files", f.OrigPath),
			})
		}

		// 检查是否触碰 forbidden_files
		if v.isForbidden(f.OrigPath) || v.isForbidden(f.NewPath) {
			errs = append(errs, &ValidationError{
				File:    f.OrigPath,
				Message: fmt.Sprintf("file %q is in forbidden_files", f.OrigPath),
			})
		}

		// 检查是否删除文件
		if f.IsDelete {
			errs = append(errs, &ValidationError{
				File:    f.OrigPath,
				Message: "file deletion is not allowed in patch-only mode",
			})
		}

		// 检查是否为二进制文件
		if isBinaryPath(f.OrigPath) {
			errs = append(errs, &ValidationError{
				File:    f.OrigPath,
				Message: "binary file modification is not allowed",
			})
		}
	}

	return errs
}

func (v *Validator) isAllowed(path string) bool {
	for _, allowed := range v.allowedFiles {
		if matchPath(allowed, path) {
			return true
		}
	}
	return false
}

func (v *Validator) isForbidden(path string) bool {
	for _, forbidden := range v.forbiddenFiles {
		if matchPath(forbidden, path) {
			return true
		}
	}
	return false
}

// matchPath 使用 glob 匹配（支持 * 通配符）
func matchPath(pattern, path string) bool {
	pattern = filepath.ToSlash(filepath.Clean(filepath.FromSlash(pattern)))
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	matched, err := filepath.Match(pattern, path)
	if err != nil {
		return pattern == path
	}
	return matched
}

func isSafeRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// isBinaryPath 判断是否是二进制文件路径
func isBinaryPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".bin": true, ".dat": true, ".zip": true, ".tar": true,
		".gz": true, ".7z": true, ".rar": true, ".pfx": true,
		".p12": true, ".cer": true, ".jpg": true, ".png": true,
		".gif": true, ".ico": true, ".pdf": true, ".doc": true,
		".docx": true, ".xls": true, ".xlsx": true,
	}
	return binaryExts[ext]
}

// Applier Patch 应用器
type Applier struct {
	projectRoot string
}

// NewApplier 创建应用器
func NewApplier(projectRoot string) *Applier {
	return &Applier{projectRoot: projectRoot}
}

// Apply 应用 parsed diff 到文件系统
// 返回修改的文件列表
func (a *Applier) Apply(result *ParseResult) ([]string, error) {
	var modifiedFiles []string

	for _, f := range result.Files {
		if f.IsDelete {
			continue // 不支持删除
		}

		targetFile := f.NewPath
		if targetFile == "" {
			targetFile = f.OrigPath
		}
		targetPath, err := safeTargetPath(a.projectRoot, targetFile)
		if err != nil {
			return nil, err
		}

		if f.IsNew {
			// 创建新文件
			content, err := a.buildNewContent(f)
			if err != nil {
				return nil, fmt.Errorf("build new file %s: %w", targetPath, err)
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return nil, fmt.Errorf("create directory for %s: %w", targetPath, err)
			}
			if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("write new file %s: %w", targetPath, err)
			}
			modifiedFiles = append(modifiedFiles, targetPath)
			continue
		}

		// 修改现有文件
		origContent, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", targetPath, err)
		}

		newContent, err := a.applyHunks(string(origContent), f.Hunks)
		if err != nil {
			return nil, fmt.Errorf("apply hunks to %s: %w", targetPath, err)
		}

		if err := os.WriteFile(targetPath, []byte(newContent), 0644); err != nil {
			return nil, fmt.Errorf("write file %s: %w", targetPath, err)
		}

		modifiedFiles = append(modifiedFiles, targetPath)
	}

	return modifiedFiles, nil
}

func safeTargetPath(projectRoot, path string) (string, error) {
	if !isSafeRelativePath(path) {
		return "", fmt.Errorf("unsafe patch target path %q", path)
	}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return "", fmt.Errorf("resolve patch target %q: %w", path, err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("patch target %q escapes project root", path)
	}
	return target, nil
}

// buildNewContent 根据 hunks 构建新文件内容
func (a *Applier) buildNewContent(f DiffFile) (string, error) {
	var lines []string
	for _, hunk := range f.Hunks {
		origCount := 0
		newCount := 0
		for _, line := range hunk.Lines {
			switch {
			case strings.HasPrefix(line, "+"):
				lines = append(lines, line[1:])
				newCount++
			case strings.HasPrefix(line, "-"):
				origCount++
			case line == `\ No newline at end of file`:
				continue
			default:
				return "", fmt.Errorf("invalid new-file hunk line %q", line)
			}
		}
		if origCount != hunk.OrigLines || newCount != hunk.NewLines {
			return "", fmt.Errorf(
				"hunk line count mismatch: header -%d +%d, content -%d +%d",
				hunk.OrigLines,
				hunk.NewLines,
				origCount,
				newCount,
			)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// applyHunks 将 hunks 应用到原始文件内容
func (a *Applier) applyHunks(original string, hunks []Hunk) (string, error) {
	lineEnding := "\n"
	if strings.Contains(original, "\r\n") {
		lineEnding = "\r\n"
	}
	normalized := strings.ReplaceAll(original, "\r\n", "\n")
	origLines := strings.Split(normalized, "\n")
	var result []string
	origIdx := 0 // 0-based line index in original

	for _, hunk := range hunks {
		if hunk.OrigStart < 1 {
			return "", fmt.Errorf("invalid hunk original start line %d", hunk.OrigStart)
		}
		hunkOrigStart := hunk.OrigStart - 1 // 转为 0-based
		if hunkOrigStart < origIdx || hunkOrigStart > len(origLines) {
			return "", fmt.Errorf("hunk starts outside original file at line %d", hunk.OrigStart)
		}

		// 添加 hunk 前的不变内容
		for origIdx < hunkOrigStart && origIdx < len(origLines) {
			result = append(result, origLines[origIdx])
			origIdx++
		}

		origCount := 0
		newCount := 0
		for _, line := range hunk.Lines {
			switch {
			case strings.HasPrefix(line, " "):
				expected := line[1:]
				if origIdx >= len(origLines) || origLines[origIdx] != expected {
					return "", fmt.Errorf("context mismatch at original line %d", origIdx+1)
				}
				result = append(result, expected)
				origIdx++
				origCount++
				newCount++
			case strings.HasPrefix(line, "-"):
				expected := line[1:]
				if origIdx >= len(origLines) || origLines[origIdx] != expected {
					return "", fmt.Errorf("removed line mismatch at original line %d", origIdx+1)
				}
				origIdx++
				origCount++
			case strings.HasPrefix(line, "+"):
				result = append(result, line[1:])
				newCount++
			case line == `\ No newline at end of file`:
				continue
			default:
				return "", fmt.Errorf("invalid hunk line %q", line)
			}
		}
		if origCount != hunk.OrigLines || newCount != hunk.NewLines {
			return "", fmt.Errorf(
				"hunk line count mismatch: header -%d +%d, content -%d +%d",
				hunk.OrigLines,
				hunk.NewLines,
				origCount,
				newCount,
			)
		}
	}

	// 添加 hunk 后的剩余行
	for origIdx < len(origLines) {
		result = append(result, origLines[origIdx])
		origIdx++
	}

	return strings.Join(result, lineEnding), nil
}

// QuickValidate 快速检查响应是否是有效的 unified diff
func QuickValidate(response string) bool {
	response = strings.TrimSpace(response)

	if response == "NEED_MORE_CONTEXT" || response == "REFUSE" || response == "FAILED" {
		return false
	}

	// 检查是否包含 diff header
	hasDiffHeader := strings.Contains(response, "diff --git ") ||
		strings.Contains(response, "--- a/") ||
		strings.Contains(response, "+++ b/")

	// 检查是否包含 hunk header
	hasHunk := strings.Contains(response, "@@ -")

	return hasDiffHeader && hasHunk
}
