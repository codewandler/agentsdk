// Package markdown turns Markdown files into slash commands.
package markdown

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/codewandler/agentsdk/command"
	md "github.com/codewandler/agentsdk/markdown"
)

// Frontmatter is the YAML schema understood by Markdown commands.
type Frontmatter struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Aliases       []string `yaml:"aliases,omitempty"`
	ArgumentHint  string   `yaml:"argument-hint,omitempty"`
	UserCallable  *bool    `yaml:"user-callable,omitempty"`
	AgentCallable bool     `yaml:"agent-callable,omitempty"`
	Internal      bool     `yaml:"internal,omitempty"`
}

// Command is a command backed by a Markdown prompt template.
type Command struct {
	spec command.Descriptor
	body string
}

// New creates a Markdown command from content.
func New(name string, content []byte) (*Command, error) {
	meta, body, err := md.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}
	fm, err := md.Bind[Frontmatter](meta)
	if err != nil {
		return nil, err
	}
	if fm.Name == "" {
		fm.Name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	spec := command.Descriptor{
		Name:         fm.Name,
		Aliases:      fm.Aliases,
		Description:  fm.Description,
		ArgumentHint: fm.ArgumentHint,
		Policy: command.Policy{
			AgentCallable: fm.AgentCallable,
			Internal:      fm.Internal,
		},
	}
	if fm.UserCallable != nil {
		spec.Policy.UserCallable = *fm.UserCallable
	}
	if spec.Description == "" {
		spec.Description = fmt.Sprintf("Run %s", spec.Name)
	}
	return &Command{spec: spec, body: body}, nil
}

// FromFile creates a Markdown command from an OS file path.
func FromFile(path string) (*Command, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read command file %q: %w", path, err)
	}
	return New(filepath.Base(path), data)
}

// LoadFS loads Markdown commands from dir in fsys. Missing dirs are ignored.
func LoadFS(fsys fs.FS, dir string) ([]command.Command, error) {
	if fsys == nil {
		return nil, nil
	}
	dir = path.Clean(strings.TrimPrefix(filepath.ToSlash(dir), "/"))
	if dir == "" {
		dir = "."
	}
	var out []command.Command
	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d == nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read command %q: %w", path, err)
		}
		cmd, err := New(d.Name(), data)
		if err != nil {
			return fmt.Errorf("parse command %q: %w", path, err)
		}
		out = append(out, cmd)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}

// LoadDir loads Markdown commands from an OS directory.
func LoadDir(dir string) ([]command.Command, error) {
	return LoadFS(os.DirFS(dir), ".")
}

func (c *Command) Descriptor() command.Descriptor { return c.spec }

func (c *Command) Execute(_ context.Context, params command.Params) (command.Result, error) {
	rendered := c.render(params)
	if strings.TrimSpace(rendered) == "" {
		return command.Handled(), nil
	}
	return command.AgentTurn(rendered), nil
}

func (c *Command) render(params command.Params) string {
	tmpl, err := template.New(c.spec.Name).Parse(c.body)
	if err != nil {
		return c.body
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData(params)); err != nil {
		return c.body
	}
	return buf.String()
}

type data struct {
	Query string
	Raw   string
	Args  []string
	Flags map[string]string
}

func templateData(params command.Params) data {
	flags := params.Flags
	if flags == nil {
		flags = map[string]string{}
	}
	args := params.Args
	if args == nil {
		args = []string{}
	}
	query := flags["query"]
	if query == "" && len(args) > 0 {
		query = strings.Join(args, " ")
	}
	return data{Query: query, Raw: params.Raw, Args: args, Flags: flags}
}
