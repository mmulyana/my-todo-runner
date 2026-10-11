package agent

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"text/template"
	"time"
)

//go:embed prompt.md
var defaultPrompt string

type PromptData struct {
	ID         string
	Title      string
	Note       string
	RepoAlias  string
	Branch     string
	Base       string
	ResultPath string
}

type Result struct {
	Status   string `json:"status"`
	PRTitle  string `json:"prTitle"`
	Summary  string `json:"summary"`
	Question string `json:"question"`

	failure string
}

func (r Result) Done() bool { return r.failure == "" && r.Status == "done" }

func (r Result) Reason() string {
	if r.failure != "" {
		return r.failure
	}
	if r.Status == "blocked" {
		return "Agent butuh keputusan: " + r.Question
	}
	return "status hasil tidak dikenal: " + r.Status
}

type Runner struct {
	Bin       string
	Args      []string
	Timeout   time.Duration
	MCPConfig string
	prompt    *template.Template
}

func MCPConfigFor(url, token string) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"mcp": map[string]any{
			"my-todo": map[string]any{
				"type":    "remote",
				"url":     url,
				"headers": map[string]string{"Authorization": "Bearer " + token},
			},
		},
	})
	return string(raw), err
}

func New(bin string, args []string, timeout time.Duration, promptFile, mcpConfig string) (*Runner, error) {
	source := defaultPrompt
	if promptFile != "" {
		raw, err := os.ReadFile(promptFile)
		if err != nil {
			return nil, fmt.Errorf("baca prompt: %w", err)
		}
		source = string(raw)
	}
	tmpl, err := template.New("prompt").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse prompt: %w", err)
	}
	return &Runner{Bin: bin, Args: args, Timeout: timeout, MCPConfig: mcpConfig, prompt: tmpl}, nil
}

func (r *Runner) Run(ctx context.Context, dir string, data PromptData) Result {
	var prompt bytes.Buffer
	if err := r.prompt.Execute(&prompt, data); err != nil {
		return Result{failure: "render prompt gagal: " + err.Error()}
	}

	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	args := append(append([]string{}, r.Args...), prompt.String())
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		// note: opencode menentukan folder project dari PWD warisan, bukan cmd.Dir
		"PWD="+dir,
		"AGENT_RESULT_PATH="+data.ResultPath,
		"MY_TODO_TODO_ID="+data.ID,
		// note: MCP disuntik lewat env biar token nggak perlu disimpan lagi di config opencode VPS
		"OPENCODE_CONFIG_CONTENT="+r.MCPConfig,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	isolate(cmd)
	cmd.WaitDelay = 30 * time.Second

	runErr := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Result{failure: fmt.Sprintf("timeout setelah %s", r.Timeout)}
	}

	raw, err := os.ReadFile(data.ResultPath)
	if err != nil {
		return Result{failure: fmt.Sprintf("agent selesai tanpa menulis file hasil (exit: %v)", runErr)}
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{failure: "file hasil bukan JSON valid: " + err.Error()}
	}
	if res.Status != "done" && res.Status != "blocked" {
		return Result{failure: fmt.Sprintf("status hasil tidak dikenal: %q", res.Status)}
	}
	return res
}
