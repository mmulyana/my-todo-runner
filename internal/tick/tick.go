package tick

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
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
	card, project := t.pick(todos)
	if card == nil {
		log.Info("tidak ada kartu siap", "assignee", cfg.Assignee, "kandidat", len(todos))
		return nil
	}
	log = log.With("todo", card.ID, "title", card.Title, "project", project.Name)
	t.log = log

	repo, repoErr := pickRepo(project, card.Note)
	if dryRun {
		log.Info("dry-run: kartu yang akan diambil", "column", card.KanbanColumnName, "repo", repo.Alias, "repoError", errString(repoErr))
		return nil
	}
	if repoErr != nil {
		t.block(ctx, card, repoErr.Error())
		return nil
	}

	if err := client.MoveTodo(ctx, card.ID, cfg.Columns.InProgress); err != nil {
		return fmt.Errorf("klaim kartu: %w", err)
	}
	log.Info("kartu diklaim", "repo", repo.Alias)

	wt, err := t.ws.Prepare(ctx, repo, card.ID, card.Title)
	if err != nil {
		t.block(ctx, card, "gagal menyiapkan repo: "+err.Error())
		return nil
	}

	res := t.agent.Run(ctx, wt.Path, agent.PromptData{
		ID:         card.ID,
		Title:      card.Title,
		Note:       card.Note,
		RepoAlias:  repo.Alias,
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

func (t *Tick) pick(todos []mytodo.Todo) (*mytodo.Todo, *config.Project) {
	for i := range todos {
		todo := &todos[i]
		if todo.ParentID != nil || !t.cfg.Pickable(todo.KanbanColumnName) {
			continue
		}
		project := t.cfg.Project(todo.ProjectName)
		if project == nil {
			t.log.Warn("kartu agent di project yang tidak ada di config, dilewati", "todo", todo.ID, "project", todo.ProjectName)
			continue
		}
		return todo, project
	}
	return nil, nil
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

var repoLine = regexp.MustCompile(`(?mi)^\s*repo:\s*([\w.-]+)\s*$`)

func pickRepo(project *config.Project, note string) (config.Repo, error) {
	if len(project.Repos) == 1 {
		return project.Repos[0], nil
	}
	aliases := make([]string, len(project.Repos))
	for i, repo := range project.Repos {
		aliases[i] = repo.Alias
	}
	match := repoLine.FindStringSubmatch(note)
	if match == nil {
		return config.Repo{}, fmt.Errorf("project %q punya beberapa repo, tambahkan baris `repo: <%s>` di note", project.Name, strings.Join(aliases, "|"))
	}
	for _, repo := range project.Repos {
		if strings.EqualFold(repo.Alias, match[1]) {
			return repo, nil
		}
	}
	return config.Repo{}, fmt.Errorf("repo %q tidak ada di project %q (pilihan: %s)", match[1], project.Name, strings.Join(aliases, ", "))
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
