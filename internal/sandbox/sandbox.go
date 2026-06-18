// Package sandbox 提供 Git 和文件级快照的创建与回滚功能。
// 在任何写入操作前必须创建保护点。
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var safeTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// GitDetector 检测 Git 仓库状态
type GitDetector struct {
	projectRoot string
}

// NewGitDetector 创建 Git 检测器
func NewGitDetector(projectRoot string) *GitDetector {
	return &GitDetector{projectRoot: projectRoot}
}

// IsGitRepo 检查是否是 Git 仓库
func (d *GitDetector) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = d.projectRoot
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// IsClean 检查工作区是否干净（没有未提交的更改）
func (d *GitDetector) IsClean() (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = d.projectRoot
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(path, ".coding-bridge/") || strings.HasPrefix(path, ".coding-bridge\\") {
			continue
		}
		return false, nil
	}
	return true, nil
}

// GetCurrentBranch 获取当前分支名
func (d *GitDetector) GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = d.projectRoot
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// GitWorktree Git worktree 管理
type GitWorktree struct {
	projectRoot string
	worktreeDir string
}

// NewGitWorktree 创建 worktree 管理器
func NewGitWorktree(projectRoot string) *GitWorktree {
	return &GitWorktree{
		projectRoot: projectRoot,
		worktreeDir: filepath.Join(projectRoot, ".coding-bridge", "worktrees"),
	}
}

// Create 创建一个新的 worktree
func (w *GitWorktree) Create(taskID string) (string, string, error) {
	branchName := fmt.Sprintf("bridge/%s", taskID)
	worktreePath := filepath.Join(w.worktreeDir, taskID)

	// 确保目录存在
	if err := os.MkdirAll(w.worktreeDir, 0755); err != nil {
		return "", "", fmt.Errorf("create worktree dir: %w", err)
	}

	// 创建 worktree
	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branchName)
	cmd.Dir = w.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("git worktree add: %w\n%s", err, string(output))
	}

	return worktreePath, branchName, nil
}

// Remove 移除 worktree
func (w *GitWorktree) Remove(taskID string) error {
	worktreePath := filepath.Join(w.worktreeDir, taskID)

	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = w.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, string(output))
	}

	return nil
}

// List 列出所有 worktree
func (w *GitWorktree) List() ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = w.projectRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var worktrees []string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			worktrees = append(worktrees, strings.TrimPrefix(line, "worktree "))
		}
	}
	return worktrees, nil
}

// Prune 清理无效的 worktree
func (w *GitWorktree) Prune() error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = w.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree prune: %w\n%s", err, string(output))
	}
	return nil
}

// GitBranch Git 分支管理
type GitBranch struct {
	projectRoot string
}

// NewGitBranch 创建分支管理器
func NewGitBranch(projectRoot string) *GitBranch {
	return &GitBranch{projectRoot: projectRoot}
}

// CreateAndSwitch 创建并切换到新分支
func (b *GitBranch) CreateAndSwitch(taskID string) (string, error) {
	branchName := fmt.Sprintf("bridge/%s", taskID)

	cmd := exec.Command("git", "checkout", "-b", branchName)
	cmd.Dir = b.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git checkout -b: %w\n%s", err, string(output))
	}

	return branchName, nil
}

// SwitchBack 切回原分支
func (b *GitBranch) SwitchBack(branchName string) error {
	cmd := exec.Command("git", "checkout", branchName)
	cmd.Dir = b.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout: %w\n%s", err, string(output))
	}
	return nil
}

// DeleteBranch 删除分支
func (b *GitBranch) DeleteBranch(branchName string) error {
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = b.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git branch -D: %w\n%s", err, string(output))
	}
	return nil
}

// GitStash Git stash 管理（无 worktree 支持时的后备方案）
type GitStash struct {
	projectRoot string
}

// NewGitStash 创建 stash 管理器
func NewGitStash(projectRoot string) *GitStash {
	return &GitStash{projectRoot: projectRoot}
}

// StashPush 暂存当前更改
func (s *GitStash) StashPush(taskID string) error {
	message := fmt.Sprintf("coding-bridge: %s", taskID)
	cmd := exec.Command("git", "stash", "push", "-m", message)
	cmd.Dir = s.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash push: %w\n%s", err, string(output))
	}
	return nil
}

// StashPop 恢复暂存
func (s *GitStash) StashPop() error {
	cmd := exec.Command("git", "stash", "pop")
	cmd.Dir = s.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash pop: %w\n%s", err, string(output))
	}
	return nil
}

// SnapshotManager 快照管理器 —— 统一入口
type SnapshotManager struct {
	projectRoot string
	snapshotDir string
	detector    *GitDetector
	worktree    *GitWorktree
	branch      *GitBranch
	stash       *GitStash
	bak         *BakSnapshot
}

// SnapshotMethod 快照方法
type SnapshotMethod string

const (
	MethodWorktree SnapshotMethod = "worktree"
	MethodBranch   SnapshotMethod = "branch"
	MethodStash    SnapshotMethod = "stash"
	MethodBak      SnapshotMethod = "bak"
)

// Snapshot 快照信息
type Snapshot struct {
	TaskID       string         `json:"task_id"`
	Method       SnapshotMethod `json:"method"`
	WorktreePath string         `json:"worktree_path,omitempty"`
	BranchName   string         `json:"branch_name,omitempty"`
	CreatedFiles []string       `json:"created_files,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// NewSnapshotManager 创建快照管理器
func NewSnapshotManager(projectRoot string) *SnapshotManager {
	return &SnapshotManager{
		projectRoot: projectRoot,
		snapshotDir: filepath.Join(projectRoot, ".coding-bridge", "snapshots"),
		detector:    NewGitDetector(projectRoot),
		worktree:    NewGitWorktree(projectRoot),
		branch:      NewGitBranch(projectRoot),
		stash:       NewGitStash(projectRoot),
		bak:         NewBakSnapshot(projectRoot),
	}
}

// CreateSnapshot 创建快照（自动选择最佳方法）
func (sm *SnapshotManager) CreateSnapshot(taskID string) (*Snapshot, error) {
	if !safeTaskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task ID %q", taskID)
	}
	snapshot := &Snapshot{
		TaskID:    taskID,
		CreatedAt: time.Now(),
	}

	if !sm.detector.IsGitRepo() {
		snapshot.Method = MethodBak
		return snapshot, nil
	}

	// 脏工作区不能安全地复制到基于 HEAD 的 worktree，改用文件级备份。
	clean, err := sm.detector.IsClean()
	if err != nil || !clean {
		snapshot.Method = MethodBak
		return snapshot, nil
	}

	if worktreePath, branchName, err := sm.worktree.Create(taskID); err == nil {
		snapshot.Method = MethodWorktree
		snapshot.WorktreePath = worktreePath
		snapshot.BranchName = branchName
		return snapshot, nil
	}

	snapshot.Method = MethodBak
	return snapshot, nil
}

// PrepareSnapshot 在写入前备份 Bak 模式涉及的文件。
func (sm *SnapshotManager) PrepareSnapshot(snapshot *Snapshot, files []string) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	if snapshot.Method != MethodBak {
		return sm.SaveSnapshot(snapshot)
	}

	var existing []string
	for _, file := range files {
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(sm.projectRoot, filepath.FromSlash(file))
		}
		info, err := os.Stat(path)
		switch {
		case err == nil && !info.IsDir():
			existing = append(existing, path)
		case os.IsNotExist(err):
			snapshot.CreatedFiles = append(snapshot.CreatedFiles, path)
		case err != nil:
			return fmt.Errorf("inspect snapshot target %s: %w", file, err)
		}
	}

	if len(existing) == 0 {
		return sm.SaveSnapshot(snapshot)
	}
	if _, err := sm.bak.BackupFiles(snapshot.TaskID, existing); err != nil {
		return err
	}
	return sm.SaveSnapshot(snapshot)
}

// ExecutionRoot 返回 patch 和命令实际运行的目录。
func (sm *SnapshotManager) ExecutionRoot(snapshot *Snapshot) string {
	if snapshot != nil && snapshot.Method == MethodWorktree && snapshot.WorktreePath != "" {
		return snapshot.WorktreePath
	}
	return sm.projectRoot
}

// Rollback 回滚到快照状态
func (sm *SnapshotManager) Rollback(snapshot *Snapshot) error {
	if snapshot == nil {
		return fmt.Errorf("snapshot is required")
	}
	var err error
	switch snapshot.Method {
	case MethodWorktree:
		if err = sm.worktree.Remove(snapshot.TaskID); err == nil && snapshot.BranchName != "" {
			err = sm.branch.DeleteBranch(snapshot.BranchName)
		}
	case MethodBranch:
		// 切回原分支并删除任务分支
		var origBranch string
		origBranch, err = sm.detector.GetCurrentBranch()
		if err == nil {
			err = sm.branch.SwitchBack(origBranch)
		}
		if err == nil {
			err = sm.branch.DeleteBranch(snapshot.BranchName)
		}
	case MethodStash:
		err = sm.stash.StashPop()
	case MethodBak:
		for _, path := range snapshot.CreatedFiles {
			if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
				err = fmt.Errorf("remove created file %s: %w", path, removeErr)
				break
			}
		}
		if err == nil {
			err = sm.bak.Restore(snapshot.TaskID)
		}
	default:
		err = fmt.Errorf("unknown snapshot method: %s", snapshot.Method)
	}
	if err != nil {
		return err
	}
	_ = os.Remove(sm.snapshotPath(snapshot.TaskID))
	return nil
}

// Cleanup 清理快照（任务成功后）
func (sm *SnapshotManager) Cleanup(snapshot *Snapshot) error {
	// 成功后的 worktree 和 Bak 均保留，供 Controller 审查或用户回滚。
	return nil
}

// SaveSnapshot 持久化快照元数据，供后续 CLI 回滚使用。
func (sm *SnapshotManager) SaveSnapshot(snapshot *Snapshot) error {
	if snapshot == nil || !safeTaskIDPattern.MatchString(snapshot.TaskID) {
		return fmt.Errorf("invalid snapshot task ID")
	}
	if err := os.MkdirAll(sm.snapshotDir, 0755); err != nil {
		return fmt.Errorf("create snapshot metadata dir: %w", err)
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot metadata: %w", err)
	}
	if err := os.WriteFile(sm.snapshotPath(snapshot.TaskID), data, 0644); err != nil {
		return fmt.Errorf("write snapshot metadata: %w", err)
	}
	return nil
}

// LoadSnapshot 读取指定任务的快照元数据。
func (sm *SnapshotManager) LoadSnapshot(taskID string) (*Snapshot, error) {
	if !safeTaskIDPattern.MatchString(taskID) {
		return nil, fmt.Errorf("invalid task ID %q", taskID)
	}
	data, err := os.ReadFile(sm.snapshotPath(taskID))
	if err != nil {
		return nil, fmt.Errorf("read snapshot metadata for %s: %w", taskID, err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("parse snapshot metadata for %s: %w", taskID, err)
	}
	return &snapshot, nil
}

func (sm *SnapshotManager) snapshotPath(taskID string) string {
	return filepath.Join(sm.snapshotDir, taskID+".json")
}

// BakSnapshot 文件级 Bak 快照
type BakSnapshot struct {
	projectRoot string
	backupDir   string
}

// NewBakSnapshot 创建 Bak 快照管理器
func NewBakSnapshot(projectRoot string) *BakSnapshot {
	return &BakSnapshot{
		projectRoot: projectRoot,
		backupDir:   filepath.Join(projectRoot, ".coding-bridge", "backups"),
	}
}

// BackupFiles 备份指定文件
func (b *BakSnapshot) BackupFiles(taskID string, files []string) (string, error) {
	taskBackupDir := filepath.Join(b.backupDir, taskID, "files")
	if err := os.MkdirAll(taskBackupDir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("read file for backup %s: %w", f, err)
		}

		// 保持相对路径结构
		relPath, err := filepath.Rel(b.projectRoot, f)
		if err != nil {
			relPath = filepath.Base(f)
		}

		backupPath := filepath.Join(taskBackupDir, relPath)
		if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
			return "", fmt.Errorf("create backup subdir: %w", err)
		}

		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return "", fmt.Errorf("write backup file: %w", err)
		}
	}

	return taskBackupDir, nil
}

// Restore 从 Bak 恢复文件
func (b *BakSnapshot) Restore(taskID string) error {
	taskBackupDir := filepath.Join(b.backupDir, taskID, "files")
	if _, err := os.Stat(taskBackupDir); os.IsNotExist(err) {
		return nil
	}

	return filepath.Walk(taskBackupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(taskBackupDir, path)
		if err != nil {
			return err
		}

		origPath := filepath.Join(b.projectRoot, relPath)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read backup: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(origPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(origPath, data, 0644)
	})
}

// getGitDiff 获取当前工作区 diff
func GetGitDiff(projectRoot string) (string, error) {
	cmd := exec.Command("git", "diff")
	cmd.Dir = projectRoot
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(output), nil
}
