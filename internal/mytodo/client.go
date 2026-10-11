package mytodo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Todo struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Note             string  `json:"note"`
	Completed        bool    `json:"completed"`
	ParentID         *string `json:"parentId"`
	ProjectID        *string `json:"projectId"`
	ProjectName      string  `json:"projectName"`
	KanbanColumnName string  `json:"kanbanColumnName"`
	Assignee         *string `json:"assignee"`
}

type Client struct {
	session *mcp.ClientSession
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

func Connect(ctx context.Context, url, token string) (*Client, error) {
	transport := &mcp.StreamableClientTransport{
		Endpoint: url,
		HTTPClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: bearerTransport{token: token, base: http.DefaultTransport},
		},
		// note: server my-todo stateless dan menolak GET dengan 405, jadi stream SSE terpisah dimatikan
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "my-todo-runner", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect MCP: %w", err)
	}
	return &Client{session: session}, nil
}

func (c *Client) Close() error {
	return c.session.Close()
}

func (c *Client) call(ctx context.Context, tool string, args map[string]any, out any) error {
	res, err := c.session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	if res.IsError {
		return fmt.Errorf("%s: %s", tool, text(res))
	}
	if out == nil {
		return nil
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	return json.Unmarshal(raw, out)
}

func text(res *mcp.CallToolResult) string {
	var parts []string
	for _, content := range res.Content {
		if t, ok := content.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	if len(parts) == 0 {
		return "unknown error"
	}
	return strings.Join(parts, "\n")
}

// note: server mengembalikan todo top-level urut createdAt, tick bergantung pada urutan ini untuk ambil kartu terlama
func (c *Client) ListAgentTodos(ctx context.Context, assignee string) ([]Todo, error) {
	var out struct {
		Todos []Todo `json:"todos"`
	}
	err := c.call(ctx, "list_todos", map[string]any{
		"assignee":  assignee,
		"completed": false,
		"limit":     100,
	}, &out)
	return out.Todos, err
}

func (c *Client) GetTodo(ctx context.Context, id string) (Todo, error) {
	var todo Todo
	err := c.call(ctx, "get_todo", map[string]any{"id": id}, &todo)
	return todo, err
}

func (c *Client) MoveTodo(ctx context.Context, id, column string) error {
	return c.call(ctx, "move_todo", map[string]any{"id": id, "columnName": column}, nil)
}

func (c *Client) AddAttachment(ctx context.Context, todoID, url string) error {
	return c.call(ctx, "add_attachment", map[string]any{"todoId": todoID, "url": url}, nil)
}

// note: update_todo mengganti note utuh, jadi note lama diambil dulu lalu disambung
func (c *Client) AppendNote(ctx context.Context, id, body string) error {
	todo, err := c.GetTodo(ctx, id)
	if err != nil {
		return err
	}
	section := fmt.Sprintf("--- agent %s ---\n%s", time.Now().Format("2006-01-02 15:04"), body)
	note := section
	if strings.TrimSpace(todo.Note) != "" {
		note = strings.TrimRight(todo.Note, "\n") + "\n\n" + section
	}
	return c.call(ctx, "update_todo", map[string]any{"id": id, "note": note}, nil)
}
