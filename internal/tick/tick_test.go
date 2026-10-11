package tick

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/mmulyana/my-todo-runner/internal/config"
	"github.com/mmulyana/my-todo-runner/internal/mytodo"
)

const (
	be = "https://github.com/mmulyana/my-todo-be"
	fe = "https://github.com/mmulyana/my-todo-fe"
)

func ptr(s string) *string { return &s }

func TestPickRepo(t *testing.T) {
	cases := []struct {
		name    string
		repo    *string
		repos   []string
		want    string
		wantErr string
	}{
		{name: "satu repo, tidak dipilih", repos: []string{be}, want: be},
		{name: "dipilih", repo: ptr(fe), repos: []string{be, fe}, want: fe},
		{name: "banyak repo, tidak dipilih", repos: []string{be, fe}, wantErr: "pilih repo di detail todo"},
		{name: "project tanpa repo", repos: nil, wantErr: "belum punya repo"},
		{name: "repo sudah dihapus dari project", repo: ptr(fe), repos: []string{be}, wantErr: "tidak ada lagi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pickRepo(&mytodo.Todo{ProjectName: "my-todo", Repo: tc.repo}, tc.repos)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestPick(t *testing.T) {
	parent := "p1"
	cfg := &config.Config{Columns: config.Columns{PickFrom: []string{"Todo"}}}
	tk := &Tick{cfg: cfg, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	todos := []mytodo.Todo{
		{ID: "in-progress", ProjectName: "my-todo", KanbanColumnName: "In Progress"},
		{ID: "subtodo", ProjectName: "my-todo", KanbanColumnName: "Todo", ParentID: &parent},
		{ID: "no-project", KanbanColumnName: "Todo"},
		{ID: "ready", ProjectName: "my-todo", KanbanColumnName: "Todo"},
		{ID: "ready-later", ProjectName: "lain", KanbanColumnName: "Todo"},
	}
	if card := tk.pick(todos); card == nil || card.ID != "ready" {
		t.Fatalf("got %v", card)
	}
	if card := tk.pick(todos[:3]); card != nil {
		t.Fatalf("expected nothing pickable, got %s", card.ID)
	}
}
