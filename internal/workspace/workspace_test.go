package workspace

import "testing"

func TestRepoNames(t *testing.T) {
	url := "https://github.com/mmulyana/my-todo-be"
	if got := RepoName(url); got != "my-todo-be" {
		t.Fatalf("RepoName = %q", got)
	}
	if got := repoDirName(url); got != "mmulyana-my-todo-be" {
		t.Fatalf("repoDirName = %q", got)
	}
}
