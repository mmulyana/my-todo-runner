package tick

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mmulyana/my-todo-runner/internal/config"
	"github.com/mmulyana/my-todo-runner/internal/mytodo"
)

var myTodo = config.Project{
	Name: "my-todo",
	Repos: []config.Repo{
		{Alias: "be", URL: "https://example.com/be.git", Base: "master"},
		{Alias: "fe", URL: "https://example.com/fe.git", Base: "main"},
	},
}

func TestPickRepo(t *testing.T) {
	single := config.Project{Name: "solo", Repos: myTodo.Repos[:1]}
	if repo, err := pickRepo(&single, ""); err != nil || repo.Alias != "be" {
		t.Fatalf("single repo: got %v %v", repo, err)
	}
	if repo, err := pickRepo(&myTodo, "kerjakan filter\nRepo: FE\n"); err != nil || repo.Alias != "fe" {
		t.Fatalf("repo line: got %v %v", repo, err)
	}
	if _, err := pickRepo(&myTodo, "tanpa baris repo"); err == nil || !strings.Contains(err.Error(), "be|fe") {
		t.Fatalf("missing repo line: got %v", err)
	}
	if _, err := pickRepo(&myTodo, "repo: rn"); err == nil {
		t.Fatal("unknown alias should fail")
	}
}

func TestPick(t *testing.T) {
	parent := "p1"
	cfg := &config.Config{
		Columns:  config.Columns{PickFrom: []string{"Ready"}},
		Projects: []config.Project{myTodo},
	}
	tk := &Tick{cfg: cfg, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	todos := []mytodo.Todo{
		{ID: "in-progress", ProjectName: "my-todo", KanbanColumnName: "In Progress"},
		{ID: "subtodo", ProjectName: "my-todo", KanbanColumnName: "Ready", ParentID: &parent},
		{ID: "other-project", ProjectName: "lain", KanbanColumnName: "Ready"},
		{ID: "ready", ProjectName: "my-todo", KanbanColumnName: "Ready"},
		{ID: "ready-later", ProjectName: "my-todo", KanbanColumnName: "Ready"},
	}
	card, project := tk.pick(todos)
	if card == nil || card.ID != "ready" || project.Name != "my-todo" {
		t.Fatalf("got %v", card)
	}
	if card, _ := tk.pick(todos[:3]); card != nil {
		t.Fatalf("expected nothing pickable, got %s", card.ID)
	}
}
