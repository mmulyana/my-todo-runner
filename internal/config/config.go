package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Columns struct {
	PickFrom   []string `json:"pickFrom"`
	InProgress string   `json:"inProgress"`
	Review     string   `json:"review"`
	Blocked    string   `json:"blocked"`
}

type Config struct {
	Columns        Columns  `json:"columns"`
	Assignee       string   `json:"assignee"`
	WorkDir        string   `json:"workDir"`
	OpencodeBin    string   `json:"opencodeBin"`
	OpencodeArgs   []string `json:"opencodeArgs"`
	TimeoutMinutes int      `json:"timeoutMinutes"`
	PromptFile     string   `json:"promptFile"`

	// note: secret dibaca dari env (EnvironmentFile systemd), bukan dari config.json
	MCPURL string `json:"-"`
	Token  string `json:"-"`
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("baca config: %w", err)
	}

	cfg := Config{
		Columns: Columns{
			PickFrom:   []string{"Todo"},
			InProgress: "In Progress",
			Review:     "Review",
			Blocked:    "Blocked",
		},
		Assignee:       "agent",
		WorkDir:        "/srv/agent",
		OpencodeBin:    "opencode",
		OpencodeArgs:   []string{"run", "--auto"},
		TimeoutMinutes: 45,
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// note: trim karena file env yang tersimpan CRLF bikin header Authorization invalid
	cfg.MCPURL = strings.TrimSpace(os.Getenv("MY_TODO_MCP_URL"))
	cfg.Token = strings.TrimSpace(os.Getenv("MY_TODO_TOKEN"))

	return &cfg, cfg.validate()
}

func (c *Config) validate() error {
	var errs []error
	if c.MCPURL == "" {
		errs = append(errs, errors.New("env MY_TODO_MCP_URL kosong"))
	}
	if c.Token == "" {
		errs = append(errs, errors.New("env MY_TODO_TOKEN kosong"))
	}
	return errors.Join(errs...)
}

func (c *Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

func (c *Config) Pickable(column string) bool {
	for _, name := range c.Columns.PickFrom {
		if name == column {
			return true
		}
	}
	return false
}
