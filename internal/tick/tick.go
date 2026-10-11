package tick

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mmulyana/my-todo-runner/internal/agent"
	"github.com/mmulyana/my-todo-runner/internal/config"
	"github.com/mmulyana/my-todo-runner/internal/github"
	"github.com/mmulyana/my-todo-runner/internal/mytodo"
	"github.com/mmulyana/my-todo-runner/internal/workspace"
)

const blockedWorktreeMaxAge = 7 * 24 * time.Hour

type Tick struct {
	cfg   *config.Config
	todo  *mytodo.Client
	ws    workspace.Manager
	agent *agent.Runner
	log   *slog.Logger
}

// note: satu tick maksimal satu kartu, sisanya menunggu tick berikutnya biar context agent selalu bersih
func Run(ctx context.Context, cfg *config.Config, dryRun bool, log *slog.Logger) error {
	mcpConfig, err := agent.MCPConfigFor(cfg.MCPURL, cfg.Token)
	if err != nil {
		return err
	}
	runner, err := agent.New(cfg.OpencodeBin, cfg.OpencodeArgs, cfg.Timeout(), cfg.PromptFile, mcpConfig)
	if err != nil {
		return err
	}
	client, err := mytodo.Connect(ctx, cfg.MCPURL, cfg.Token)
	if err != nil {
		return err
	}
	defer client.Close()

	t := &Tick{cfg: cfg, todo: client, ws: workspace.Manager{WorkDir: cfg.WorkDir}, agent: runner, log: log}
	if !dryRun {
		t.ws.Prune(ctx, blockedWorktreeMaxAge)
	}

	todos, err := client.ListAgentTodos(ctx, cfg.Assignee)
	if err != nil {
		return err
	}
	card := t.pick(todos)
	if card == nil {
		log.Info("tidak ada kartu siap", "assignee", cfg.Assignee, "kandidat", len(todos))
		return nil
	}
	log = log.With("todo", card.ID, "title", card.Title, "project", card.ProjectName)
	t.log = log

	projectRepos, err := client.ProjectRepos(ctx, card.ProjectName)
	if err != nil {
		return err
	}
	repo, repoErr := pickRepo(card, projectRepos)
	if dryRun {
		log.Info("dry-run: kartu yang akan diambil", "column", card.KanbanColumnName, "repo", repo, "repoError", errString(repoErr))
		return nil
	}

	if err := t.ensureColumns(ctx, card.ProjectName); err != nil {
		return err
	}
	if repoErr != nil {
		t.block(ctx, card, repoErr.Error())
		return nil
	}

	if err := client.MoveTodo(ctx, card.ID, cfg.Columns.InProgress); err != nil {
		return fmt.Errorf("klaim kartu: %w", err)
	}
	log.Info("kartu diklaim", "repo", repo)

	wt, err := t.ws.Prepare(ctx, repo, card.ID, card.Title)
	if err != nil {
		t.block(ctx, card, "gagal menyiapkan repo: "+err.Error())
		return nil
	}

	res := t.agent.Run(ctx, wt.Path, agent.PromptData{
		ID:         card.ID,
		Title:      card.Title,
		Note:       card.Note,
		RepoName:   workspace.RepoName(repo),
		Branch:     wt.Branch,
		Base:       wt.Base,
		ResultPath: wt.ResultPath,
	})
	if !res.Done() {
		t.block(ctx, card, res.Reason())
		return nil
	}

	hasCommits, err := t.ws.HasNewCommits(ctx, wt)
	if err != nil || !hasCommits {
		t.block(ctx, card, "agent melapor selesai tapi tidak ada commit baru di "+wt.Branch)
		return nil
	}
	if err := t.ws.Push(ctx, wt); err != nil {
		t.block(ctx, card, "push gagal: "+err.Error())
		return nil
	}
	title := res.PRTitle
	if title == "" {
		title = card.Title
	}
	url, err := github.CreatePR(ctx, wt.Path, wt.Base, wt.Branch, title, prBody(card, res.Summary))
	if err != nil {
		t.block(ctx, card, "branch sudah di-push ("+wt.Branch+") tapi buka PR gagal: "+err.Error())
		return nil
	}

	if err := client.AddAttachment(ctx, card.ID, url); err != nil {
		log.Warn("attach PR gagal", "err", err)
	}
	if err := client.AppendNote(ctx, card.ID, "PR dibuka: "+url+"\n\n"+res.Summary); err != nil {
		log.Warn("update note gagal", "err", err)
	}
	if err := client.MoveTodo(ctx, card.ID, cfg.Columns.Review); err != nil {
		log.Warn("pindah ke review gagal", "err", err)
	}
	t.ws.Remove(ctx, wt)
	log.Info("selesai, PR dibuka", "url", url)
	return nil
}

func (t *Tick) pick(todos []mytodo.Todo) *mytodo.Todo {
	for i := range todos {
		todo := &todos[i]
		if todo.ParentID == nil && todo.ProjectName != "" && t.cfg.Pickable(todo.KanbanColumnName) {
			return todo
		}
	}
	return nil
}

// note: project baru cuma punya kolom Todo/In Progress/Done, jadi kolom tujuan runner dibuat saat pertama dibutuhkan
func (t *Tick) ensureColumns(ctx context.Context, project string) error {
	existing, err := t.todo.ColumnNames(ctx, project)
	if err != nil {
		return err
	}
	for _, name := range []string{t.cfg.Columns.InProgress, t.cfg.Columns.Review, t.cfg.Columns.Blocked} {
		if existing[name] {
			continue
		}
		if err := t.todo.CreateColumn(ctx, project, name); err != nil {
			return fmt.Errorf("buat kolom %q: %w", name, err)
		}
		existing[name] = true
		t.log.Info("kolom dibuat", "column", name)
	}
	return nil
}

func (t *Tick) block(ctx context.Context, card *mytodo.Todo, reason string) {
	// note: tetap jalan walau ctx dibatalkan (SIGTERM), biar kartu nggak nyangkut di In Progress
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	t.log.Warn("kartu diblokir", "reason", reason)
	if err := t.todo.AppendNote(ctx, card.ID, "Blocked: "+reason); err != nil {
		t.log.Error("update note gagal", "err", err)
	}
	if err := t.todo.MoveTodo(ctx, card.ID, t.cfg.Columns.Blocked); err != nil {
		t.log.Error("pindah ke blocked gagal", "err", err)
	}
}

func pickRepo(card *mytodo.Todo, projectRepos []string) (string, error) {
	if card.Repo != nil {
		for _, url := range projectRepos {
			if url == *card.Repo {
				return url, nil
			}
		}
		return "", fmt.Errorf("repo %s tidak ada lagi di project %q, pilih ulang repo di detail todo", *card.Repo, card.ProjectName)
	}
	switch len(projectRepos) {
	case 0:
		return "", fmt.Errorf("project %q belum punya repo, tambahkan di halaman project (Repositories)", card.ProjectName)
	case 1:
		return projectRepos[0], nil
	default:
		return "", fmt.Errorf("project %q punya %d repo, pilih repo di detail todo", card.ProjectName, len(projectRepos))
	}
}

func prBody(card *mytodo.Todo, summary string) string {
	return fmt.Sprintf("%s\n\n---\nDikerjakan agent dari todo `%s` (%s).", summary, card.Title, card.ID)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
