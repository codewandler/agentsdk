package configplugin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/command"
	"github.com/invopop/jsonschema"
)

// ConfigSchemaPayload renders the appconfig JSON Schema.
type ConfigSchemaPayload struct {
	Schema *jsonschema.Schema
}

func (p ConfigSchemaPayload) Display(mode command.DisplayMode) (string, error) {
	if p.Schema == nil {
		return "No schema available.", nil
	}
	if mode == command.DisplayJSON {
		data, err := json.MarshalIndent(p.Schema, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return renderSchemaMarkdown(p.Schema), nil
}

// renderSchemaMarkdown produces a readable markdown summary of the JSON Schema.
func renderSchemaMarkdown(root *jsonschema.Schema) string {
	var b strings.Builder
	b.WriteString("# agentsdk App Config Schema\n")

	// Resolve the root type through $ref.
	resolved := root
	if root.Ref != "" && root.Definitions != nil {
		refKey := refName(root.Ref)
		if def, ok := root.Definitions[refKey]; ok {
			resolved = def
		}
	}

	// Render top-level properties.
	if resolved.Properties != nil {
		writePropertiesMarkdown(&b, resolved, root, 2)
	}

	// Render supporting definitions.
	if root.Definitions != nil {
		names := make([]string, 0, len(root.Definitions))
		for name := range root.Definitions {
			if root.Ref != "" && name == refName(root.Ref) {
				continue // skip root, already rendered
			}
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			def := root.Definitions[name]
			fmt.Fprintf(&b, "\n## %s\n\n", name)
			if def.Description != "" {
				fmt.Fprintf(&b, "%s\n\n", def.Description)
			}
			if def.Properties != nil {
				writePropertiesMarkdown(&b, def, root, 0)
			}
		}
	}

	return b.String()
}

func writePropertiesMarkdown(b *strings.Builder, schema *jsonschema.Schema, root *jsonschema.Schema, headingLevel int) {
	if headingLevel > 0 {
		b.WriteString("\n")
	}
	b.WriteString("| Field | Type | Description |\n")
	b.WriteString("|-------|------|-------------|\n")

	keys := schemaPropertyKeys(schema)
	for _, key := range keys {
		prop := schemaPropertyByKey(schema, key)
		if prop == nil {
			continue
		}
		typeName := schemaTypeName(prop, root)
		desc := prop.Description
		if len(prop.Enum) > 0 {
			vals := make([]string, len(prop.Enum))
			for i, v := range prop.Enum {
				vals[i] = fmt.Sprintf("`%v`", v)
			}
			if desc != "" {
				desc += " "
			}
			desc += "Enum: " + strings.Join(vals, ", ")
		}
		if prop.Default != nil {
			if desc != "" {
				desc += " "
			}
			desc += fmt.Sprintf("Default: `%v`", prop.Default)
		}
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", key, typeName, desc)
	}
}

// schemaTypeName returns a human-readable type string for a property.
func schemaTypeName(prop *jsonschema.Schema, root *jsonschema.Schema) string {
	if prop.Ref != "" {
		return "`" + refName(prop.Ref) + "`"
	}
	typ := prop.Type
	if typ == "array" && prop.Items != nil {
		itemType := schemaTypeName(prop.Items, root)
		return itemType + "[]"
	}
	if typ == "" {
		return "any"
	}
	return "`" + typ + "`"
}

// refName extracts the definition name from a $ref like "#/$defs/Foo".
func refName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// schemaPropertyKeys returns sorted property names from a schema.
func schemaPropertyKeys(s *jsonschema.Schema) []string {
	if s.Properties == nil {
		return nil
	}
	keys := make([]string, 0, s.Properties.Len())
	for pair := s.Properties.Oldest(); pair != nil; pair = pair.Next() {
		keys = append(keys, pair.Key)
	}
	sort.Strings(keys)
	return keys
}

// schemaPropertyByKey looks up a property by name.
func schemaPropertyByKey(s *jsonschema.Schema, key string) *jsonschema.Schema {
	if s.Properties == nil {
		return nil
	}
	val, ok := s.Properties.Get(key)
	if !ok {
		return nil
	}
	return val
}
