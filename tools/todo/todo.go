// Package todo provides a simple in-memory todo tool.
package todo

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/codewandler/agentsdk/tool"
)

// Params are the parameters for the todo tool.
type Params struct {
	Action string `json:"action" jsonschema:"description=Action to perform: create\\, list\\, get\\, update\\, delete,required"`
	ID     int    `json:"id,omitempty" jsonschema:"description=Todo id for get\\, update\\, or delete"`
	Title  string `json:"title,omitempty" jsonschema:"description=Todo title for create or update"`
	Done   *bool  `json:"done,omitempty" jsonschema:"description=Completion status for update"`
}

// Todo is a single flat todo item.
type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

type store struct {
	Todos      []Todo
	NextTodoID int
}

var (
	mu       sync.Mutex
	sessions = map[string]*store{}
)

// Tools returns the todo tool.
func Tools() []tool.Tool { return []tool.Tool{todoTool()} }

func todoTool() tool.Tool {
	return tool.New("todo",
		"Manage a simple in-memory todo list scoped to the current session. Uses integer ids and action-based params: create, list, get, update, delete. No subtasks.",
		func(ctx tool.Ctx, p Params) (tool.Result, error) {
			action := strings.TrimSpace(strings.ToLower(p.Action))
			if action == "" {
				return tool.Error("action is required"), nil
			}

			mu.Lock()
			defer mu.Unlock()

			s := sessionStore(ctx.SessionID())

			switch action {
			case "create":
				title, err := validateTitle(p.Title)
				if err != nil {
					return tool.Error(err.Error()), nil
				}
				t := Todo{ID: s.NextTodoID, Title: title, Done: false}
				s.NextTodoID++
				s.Todos = append(s.Todos, t)
				return todoItemResult{Action: "create", Todo: t}, nil
			case "list":
				out := append([]Todo(nil), s.Todos...)
				return todoListResult{Todos: out}, nil
			case "get":
				if err := validateID(p.ID); err != nil {
					return tool.Error(err.Error()), nil
				}
				idx := findTodoIndex(s, p.ID)
				if idx < 0 {
					return tool.Error(fmt.Sprintf("todo with id %d not found", p.ID)), nil
				}
				return todoItemResult{Action: "get", Todo: s.Todos[idx]}, nil
			case "update":
				if err := validateID(p.ID); err != nil {
					return tool.Error(err.Error()), nil
				}
				if strings.TrimSpace(p.Title) == "" && p.Done == nil {
					return tool.Error("update requires at least one of title or done"), nil
				}
				idx := findTodoIndex(s, p.ID)
				if idx < 0 {
					return tool.Error(fmt.Sprintf("todo with id %d not found", p.ID)), nil
				}
				if strings.TrimSpace(p.Title) != "" {
					title, err := validateTitle(p.Title)
					if err != nil {
						return tool.Error(err.Error()), nil
					}
					s.Todos[idx].Title = title
				} else if p.Title != "" {
					return tool.Error("title must be a non-empty string"), nil
				}
				if p.Done != nil {
					s.Todos[idx].Done = *p.Done
				}
				return todoItemResult{Action: "update", Todo: s.Todos[idx]}, nil
			case "delete":
				if err := validateID(p.ID); err != nil {
					return tool.Error(err.Error()), nil
				}
				idx := findTodoIndex(s, p.ID)
				if idx < 0 {
					return tool.Error(fmt.Sprintf("todo with id %d not found", p.ID)), nil
				}
				deleted := s.Todos[idx]
				s.Todos = append(s.Todos[:idx], s.Todos[idx+1:]...)
				return todoDeleteResult{Deleted: deleted}, nil
			default:
				return tool.Error(fmt.Sprintf("unknown action: %s", p.Action)), nil
			}
		},
	)
}

func sessionStore(sessionID string) *store {
	if sessionID == "" {
		sessionID = "default"
	}
	s, ok := sessions[sessionID]
	if !ok {
		s = &store{NextTodoID: 1}
		sessions[sessionID] = s
	}
	return s
}

func validateID(id int) error {
	if id <= 0 {
		return fmt.Errorf("id must be a positive integer")
	}
	return nil
}

func validateTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("title must be a non-empty string")
	}
	return title, nil
}

func findTodoIndex(s *store, id int) int {
	for i := range s.Todos {
		if s.Todos[i].ID == id {
			return i
		}
	}
	return -1
}

func resetTodosForTest() {
	mu.Lock()
	defer mu.Unlock()
	sessions = map[string]*store{}
}

type todoItemResult struct {
	Action string `json:"action"`
	Todo   Todo   `json:"todo"`
}

func (r todoItemResult) IsError() bool { return false }
func (r todoItemResult) String() string {
	status := "open"
	if r.Todo.Done {
		status = "done"
	}
	switch r.Action {
	case "create":
		return fmt.Sprintf("Created todo #%d [%s]: %s", r.Todo.ID, status, r.Todo.Title)
	case "update":
		return fmt.Sprintf("Updated todo #%d [%s]: %s", r.Todo.ID, status, r.Todo.Title)
	default:
		return fmt.Sprintf("Todo #%d [%s]: %s", r.Todo.ID, status, r.Todo.Title)
	}
}
func (r todoItemResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type   string `json:"type"`
		Action string `json:"action"`
		Todo   Todo   `json:"todo"`
	}
	return json.Marshal(wire{Type: "todo_item", Action: r.Action, Todo: r.Todo})
}

type todoListResult struct {
	Todos []Todo `json:"todos"`
}

func (r todoListResult) IsError() bool { return false }
func (r todoListResult) String() string {
	if len(r.Todos) == 0 {
		return "No todos."
	}
	lines := make([]string, len(r.Todos))
	for i, t := range r.Todos {
		mark := " "
		if t.Done {
			mark = "x"
		}
		lines[i] = fmt.Sprintf("%d. [%s] %s", t.ID, mark, t.Title)
	}
	return strings.Join(lines, "\n")
}
func (r todoListResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type  string `json:"type"`
		Todos []Todo `json:"todos"`
	}
	return json.Marshal(wire{Type: "todo_list", Todos: r.Todos})
}

type todoDeleteResult struct {
	Deleted Todo `json:"deleted"`
}

func (r todoDeleteResult) IsError() bool { return false }
func (r todoDeleteResult) String() string {
	return fmt.Sprintf("Deleted todo #%d: %s", r.Deleted.ID, r.Deleted.Title)
}
func (r todoDeleteResult) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type    string `json:"type"`
		Deleted Todo   `json:"deleted"`
	}
	return json.Marshal(wire{Type: "todo_delete", Deleted: r.Deleted})
}
