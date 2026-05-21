package cleanup

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Worktree struct {
	Path     string
	Branch   string
	Head     string
	Bare     bool
	Detached bool
	Exists   bool
	Dirty    bool
}

type Inspector interface {
	Worktrees(repoRoot string) ([]Worktree, error)
	Branches(repoRoot string) ([]string, error)
	PathExists(path string) bool
	Dirty(path string) (bool, error)
}

type GitInspector struct {
	GitPath string
}

func (inspector GitInspector) gitPath() string {
	if strings.TrimSpace(inspector.GitPath) != "" {
		return inspector.GitPath
	}
	return "git"
}

func (inspector GitInspector) Worktrees(repoRoot string) ([]Worktree, error) {
	output, err := inspector.git(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list --porcelain failed: %w", err)
	}
	worktrees := ParseWorktreePorcelain(string(output))
	for index := range worktrees {
		worktrees[index].Exists = inspector.PathExists(worktrees[index].Path)
	}
	return worktrees, nil
}

func (inspector GitInspector) Branches(repoRoot string) ([]string, error) {
	output, err := inspector.git(repoRoot, "branch", "--format", "%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("git branch --format failed: %w", err)
	}
	return ParseBranchOutput(string(output)), nil
}

func (inspector GitInspector) PathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (inspector GitInspector) Dirty(path string) (bool, error) {
	output, err := inspector.git(path, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("git status --porcelain failed for %s: %w", path, err)
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func (inspector GitInspector) git(dir string, args ...string) ([]byte, error) {
	command := exec.Command(inspector.gitPath(), args...)
	command.Dir = dir
	return command.Output()
}

func ParseBranchOutput(output string) []string {
	seen := map[string]bool{}
	branches := []string{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		branch := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "*"))
		if branch == "" || seen[branch] {
			continue
		}
		seen[branch] = true
		branches = append(branches, branch)
	}
	sort.Strings(branches)
	return branches
}

func ParseWorktreePorcelain(output string) []Worktree {
	records := []Worktree{}
	var current Worktree
	hasCurrent := false
	flush := func() {
		if hasCurrent && strings.TrimSpace(current.Path) != "" {
			records = append(records, current)
		}
		current = Worktree{}
		hasCurrent = false
	}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			key = line
		}
		switch key {
		case "worktree":
			flush()
			current.Path = value
			hasCurrent = true
		case "HEAD":
			current.Head = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			current.Bare = true
		case "detached":
			current.Detached = true
		}
	}
	flush()
	return records
}

func ComparablePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(path))
}
