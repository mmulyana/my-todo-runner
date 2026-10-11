package github

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func gh(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh %s: %w\n%s", args[0], err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func CreatePR(ctx context.Context, dir, base, branch, title, body string) (string, error) {
	// note: PR open dipakai ulang (kartu revisi), yang sudah closed/merged diganti PR baru
	if existing, err := gh(ctx, dir, "pr", "view", branch, "--json", "url,state", "-q", `select(.state == "OPEN") | .url`); err == nil && existing != "" {
		return existing, nil
	}
	out, err := gh(ctx, dir, "pr", "create", "--base", base, "--head", branch, "--title", title, "--body", body)
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	return strings.TrimSpace(lines[len(lines)-1]), nil
}
