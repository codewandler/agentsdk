package frontmatter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_BasicFrontmatter(t *testing.T) {
	src := `---
name: my-skill
description: Does useful things
---
# Hello
`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, "my-skill", m["name"])
	require.Equal(t, "Does useful things", m["description"])
	// TrimLeft removes trailing newlines from the scanner input
	// (lines don't carry their trailing \n through the scanner).
	require.Equal(t, "# Hello", body)
}

func TestParse_MissingFrontmatter(t *testing.T) {
	src := `# Just a document

No frontmatter here.
`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.Nil(t, m)
	require.Equal(t, "# Just a document\n\nNo frontmatter here.", body)
}

func TestParse_EmptyFrontmatterBlock(t *testing.T) {
	src := `---
---
# Body`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.Nil(t, m)
	require.Equal(t, "# Body", body)
}

func TestParse_MultilineDescription(t *testing.T) {
	src := `---
description: |
  This is a
  multi-line
  description
---
# Body`
	m, _, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, "This is a\nmulti-line\ndescription", m["description"])
}

func TestParse_FrontmatterOnly(t *testing.T) {
	src := `---
name: only-frontmatter
---
`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, "only-frontmatter", m["name"])
	require.Equal(t, "", body)
}

// TestParse_HorizontalRuleInBody verifies that a bare "---" line in the body
// (a Markdown horizontal rule) does not cause the parser to re-enter frontmatter
// mode and corrupt the YAML parse.
func TestParse_HorizontalRuleInBody(t *testing.T) {
	src := `---
name: my-skill
---
# Section A

Some content.

---

# Section B

More content.
`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err, "'---' horizontal rule in body must not cause a parse error")
	require.NotNil(t, m)
	require.Equal(t, "my-skill", m["name"])
	require.Contains(t, body, "Section A")
	require.Contains(t, body, "---")
	require.Contains(t, body, "Section B")
}

func TestParse_FrontmatterWithTrailingContent(t *testing.T) {
	src := `---
name: example
---
# Title

Some content.

## Section
`
	m, body, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Equal(t, "example", m["name"])
	require.Contains(t, body, "# Title")
	require.Contains(t, body, "## Section")
}

func TestParse_NestedYAML(t *testing.T) {
	src := `---
outer:
  inner:
    value: hello
list:
  - one
  - two
---
# Body`
	m, _, err := Parse(strings.NewReader(src))
	require.NoError(t, err)
	require.NotNil(t, m)

	outer, ok := m["outer"].(map[string]any)
	require.True(t, ok, "outer should be map[string]any")
	inner, ok := outer["inner"].(map[string]any)
	require.True(t, ok, "inner should be map[string]any")
	require.Equal(t, "hello", inner["value"])

	list, ok := m["list"].([]any)
	require.True(t, ok, "list should be []any")
	require.Len(t, list, 2)
	require.Equal(t, "one", list[0])
}

func TestStripFrontmatter_Basic(t *testing.T) {
	content := `---
name: test
---
# Hello

Body.`
	m, body := StripFrontmatter(content)
	require.NotNil(t, m)
	require.Equal(t, "test", m["name"])
	require.Equal(t, "# Hello\n\nBody.", body)
}

func TestStripFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just text"
	m, body := StripFrontmatter(content)
	require.Nil(t, m)
	require.Equal(t, content, body)
}

func TestBind_StructBinding(t *testing.T) {
	type MySpec struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Aliases     []string `yaml:"aliases,omitempty"`
	}

	m := map[string]any{
		"name":        "mycmd",
		"description": "Does a thing",
		"aliases":     []any{"c", "cmd"},
	}

	got, err := Bind[MySpec](m)
	require.NoError(t, err)
	require.Equal(t, "mycmd", got.Name)
	require.Equal(t, "Does a thing", got.Description)
	require.Equal(t, []string{"c", "cmd"}, got.Aliases)
}

func TestBind_NilMap(t *testing.T) {
	type EmptySpec struct {
		Name string `yaml:"name"`
	}
	got, err := Bind[EmptySpec](nil)
	require.NoError(t, err)
	require.Equal(t, EmptySpec{}, got)
}

func TestBind_UnknownFieldsIgnored(t *testing.T) {
	type SimpleSpec struct {
		Name string `yaml:"name"`
	}
	m := map[string]any{
		"name":        "known",
		"description": "unknown field",
	}
	got, err := Bind[SimpleSpec](m)
	require.NoError(t, err)
	require.Equal(t, "known", got.Name)
}
