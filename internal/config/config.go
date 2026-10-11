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

type Repo struct {
	Alias string `json:"alias"`
	URL   string `json:"url"`
	Base  string `json:"base"`
}

type Project struct {
	Name  string `json:"name"`
	Repos []Repo `json:"repos"`
}

type Config struct {
	Columns        Columns   `json:"columns"`
	Assignee       string    `json:"assignee"`
	WorkDir        string    `json:"workDir"`
	OpencodeBin    string    `json:"opencodeBin"`
	OpencodeArgs   []string  `json:"opencodeArgs"`
	TimeoutMinutes int       `json:"timeoutMinutes"`
	PromptFile     string    `json:"promptFile"`
	Projects       []Project `json:"projects"`

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
			PickFrom:   []string{"Ready"},
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
	if len(c.Projects) == 0 {
		errs = append(errs, errors.New("projects kosong"))
	}
	for _, p := range c.Projects {
		if len(p.Repos) == 0 {
			errs = append(errs, fmt.Errorf("project %q tidak punya repo", p.Name))
		}
		for _, r := range p.Repos {
			if r.Alias == "" || r.URL == "" || r.Base == "" {
				errs = append(errs, fmt.Errorf("project %q: repo wajib punya alias, url, base", p.Name))
			}
		}
	}
	return errors.Join(errs...)
}

func (c *Config) Timeout() time.Duration {
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

func (c *Config) Project(name string) *Project {
	for i := range c.Projects {
		if c.Projects[i].Name == name {
			return &c.Projects[i]
		}
	}
	return nil
}

func (c *Config) Pickable(column string) bool {
	for _, name := range c.Columns.PickFrom {
		if name == column {
			return true
		}
	}
	return false
}
