package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mmulyana/my-todo-runner/internal/config"
)

type Worktree struct {
	Path       string
	RepoDir    string
	Branch     string
	Base       string
	ResultPath string
}

type Manager struct {
	WorkDir string
}

func (m Manager) reposDir() string { return filepath.Join(m.WorkDir, "repos") }
func (m Manager) workDir() string  { return filepath.Join(m.WorkDir, "work") }

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m Manager) Prepare(ctx context.Context, repo config.Repo, todoID, title string) (*Worktree, error) {
	for _, dir := range []string{m.reposDir(), m.workDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	repoDir := filepath.Join(m.reposDir(), repo.Alias)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		if _, err := git(ctx, m.reposDir(), "clone", repo.URL, repoDir); err != nil {
			return nil, err
		}
	} else if _, err := git(ctx, repoDir, "fetch", "origin", "--prune"); err != nil {
		return nil, err
	}

	wt := &Worktree{
		Path:       filepath.Join(m.workDir(), todoID),
		RepoDir:    repoDir,
		Branch:     "agent/" + todoID[:8] + "-" + slug(title),
		Base:       repo.Base,
		ResultPath: filepath.Join(m.workDir(), todoID+".result.json"),
	}

	// note: kartu revisi melanjutkan branch remote-nya biar commit di PR yang masih open nggak hilang saat force push
	startPoint := "origin/" + repo.Base
	if _, err := git(ctx, repoDir, "rev-parse", "--verify", "--quiet", "origin/"+wt.Branch); err == nil {
		startPoint = "origin/" + wt.Branch
	}
	m.Remove(ctx, wt)
	if _, err := git(ctx, repoDir, "worktree", "add", "-B", wt.Branch, wt.Path, startPoint); err != nil {
		return nil, err
	}
	return wt, nil
}

func (m Manager) HasNewCommits(ctx context.Context, wt *Worktree) (bool, error) {
	out, err := git(ctx, wt.Path, "rev-list", "--count", "origin/"+wt.Base+"..HEAD")
	if err != nil {
		return false, err
	}
	n, err := strconv.Atoi(out)
	return n > 0, err
}

func (m Manager) Push(ctx context.Context, wt *Worktree) error {
	// note: branch agent/* milik runner, force aman dan perlu saat kartu dikerjakan ulang
	_, err := git(ctx, wt.Path, "push", "--force", "-u", "origin", wt.Branch)
	return err
}

func (m Manager) Remove(ctx context.Context, wt *Worktree) {
	_, _ = git(ctx, wt.RepoDir, "worktree", "remove", "--force", wt.Path)
	_ = os.RemoveAll(wt.Path)
	_ = os.Remove(wt.ResultPath)
	_, _ = git(ctx, wt.RepoDir, "worktree", "prune")
}

// note: worktree kartu Blocked sengaja disimpan untuk debugging, baru dihapus setelah maxAge
func (m Manager) Prune(ctx context.Context, maxAge time.Duration) {
	entries, err := os.ReadDir(m.workDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < maxAge {
			continue
		}
		_ = os.RemoveAll(filepath.Join(m.workDir(), entry.Name()))
	}
	repos, _ := os.ReadDir(m.reposDir())
	for _, repo := range repos {
		_, _ = git(ctx, filepath.Join(m.reposDir(), repo.Name()), "worktree", "prune")
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(title string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		return "task"
	}
	return s
}
