# DESIGN: file_edit schema rework

## Problem

The `file_edit` tool schema has three structural defects.

### 1. Missing top-level `required`

`path` and `operations` are both required but neither appears in a `required`
array. Two separate root causes:

- `Operations []Op` — `Op` is a type alias (`type Op = Operation`). The
  reflector drops the `required` tag on alias-typed fields.
- `Path tool.StringSliceParam` — `StringSliceParam` implements `JSONSchema()`,
  so the reflector delegates to that method and skips the field's struct tags
  entirely, including `required`.

### 2. `operations.items` has no discriminator

Each item is a plain object with five optional sibling properties and no
`required`. There is no `oneOf` — an empty `{}` is valid. The `OpSchema` /
`JSONSchemaAlias` machinery intended to fix this is dead code: the reflector
sees `[]Operation` (concrete type, not the alias) and ignores it.

### 3. `RemoveOp` allows an empty payload

`RemoveOp` has two fields, neither required. The caller must supply
`old_string` OR `lines` but the schema gives no signal of this.

---

## Proposed changes

### 1. Export `tool.SchemaFor[P]()` and fix `required` injection

**`tool/base.go`** — two additions:

**a)** Export a `SchemaFor[P]()` helper returning just the `*jsonschema.Schema`
(the map return of `schemaFor` is internal to validation compilation):

```go
func SchemaFor[P any]() *jsonschema.Schema {
    _, s := schemaFor[P]()
    return s
}
```

**b)** Add `injectRequiredFromTags[P]` called inside `schemaFor[P]()` after
reflection, to patch `required` for fields whose types implement `JSONSchema()`
(bypassing tag processing). Uses exact token matching to avoid false positives:

```go
func injectRequiredFromTags[P any](m map[string]any) map[string]any {
    var zero P
    t := reflect.TypeOf(zero)
    if t.Kind() == reflect.Ptr {
        t = t.Elem()
    }
    if t.Kind() != reflect.Struct {
        return m
    }
    var required []any
    for i := range t.NumField() {
        f := t.Field(i)
        for _, token := range strings.Split(f.Tag.Get("jsonschema"), ",") {
            if strings.TrimSpace(token) == "required" {
                if name := strings.Split(f.Tag.Get("json"), ",")[0]; name != "" && name != "-" {
                    required = append(required, name)
                }
                break
            }
        }
    }
    if len(required) > 0 {
        m["required"] = required
    }
    return m
}
```

This is a general fix — any tool with the same tag-bypass problem benefits
automatically.

### 2. Split `RemoveOp` into two named types

Replace the current flat `RemoveOp` with two concrete types embedded as
optional pointers:

```go
type RemoveByString struct {
    OldString string `json:"old_string" jsonschema:"description=Exact text to find and remove. Mutually exclusive with lines.,required"`
}

type RemoveByLines struct {
    Lines []int `json:"lines" jsonschema:"description=Line numbers to remove (1-indexed). One element [n] removes line n. Two elements [start end] removes that inclusive range.,required"`
}

type RemoveOp struct {
    *RemoveByString
    *RemoveByLines
}
```

`RemoveOp.JSONSchema()` then uses `tool.SchemaFor[P]()` — no duplication, no
hardcoded strings, descriptions live in the canonical type definitions:

```go
func (RemoveOp) JSONSchema() *jsonschema.Schema {
    return &jsonschema.Schema{
        OneOf: []*jsonschema.Schema{
            tool.SchemaFor[RemoveByString](),
            tool.SchemaFor[RemoveByLines](),
        },
    }
}
```

Runtime JSON unmarshalling is unaffected — Go unmarshals into embedded pointer
fields naturally.

### 3. Add `Operation.JSONSchema()` for the outer discriminated union

Replace the dead `OpSchema` / `JSONSchemaAlias` machinery with
`Operation.JSONSchema()`. Uses `tool.SchemaFor[P]()` consistently — same
reflector config everywhere, no local reflector:

```go
func (Operation) JSONSchema() *jsonschema.Schema {
    makeVariant := func(key string, inner *jsonschema.Schema) *jsonschema.Schema {
        props := jsonschema.NewProperties()
        props.Set(key, inner)
        return &jsonschema.Schema{
            Properties:           props,
            Required:             []string{key},
            AdditionalProperties: jsonschema.FalseSchema,
        }
    }
    return &jsonschema.Schema{
        OneOf: []*jsonschema.Schema{
            makeVariant("replace", tool.SchemaFor[ReplaceOp]()),
            makeVariant("insert",  tool.SchemaFor[InsertOp]()),
            makeVariant("remove",  tool.SchemaFor[RemoveOp]()),  // calls RemoveOp.JSONSchema()
            makeVariant("append",  tool.SchemaFor[AppendOp]()),
            makeVariant("patch",   tool.SchemaFor[PatchOp]()),
        },
    }
}
```

### 4. Clean up `FileEditParams`

- Change `Operations []Op` → `Operations []Operation` (remove alias dependency)
- Delete `type Op = Operation`, `OpSchema`, and `JSONSchemaAlias`
- Keep `jsonschema:"required"` tags on `Path` and `Operations` — they now work
  via `injectRequiredFromTags` (Fix 1b)

---

## Per-op summary

| Op | Required fields | Optional fields | Schema fix needed |
|---|---|---|---|
| `replace` | `old_string` | `new_string`¹, `replace_all`, `if_missing` | Outer `oneOf` wrapper only |
| `insert` | `line`, `content` | `indent` | Outer `oneOf` wrapper only |
| `remove` | `old_string` OR `lines` | — | Outer wrapper + inner `oneOf` via `RemoveByString`/`RemoveByLines` |
| `append` | `content` | — | Outer `oneOf` wrapper only |
| `patch` | `patch` | — | Outer `oneOf` wrapper only |

¹ `new_string` is intentionally optional — empty string or omission deletes the matched text.

---

## Files to change

| File | Change |
|---|---|
| `tool/base.go` | Export `SchemaFor[P]()`. Add `injectRequiredFromTags[P]` called from `schemaFor[P]()`. |
| `tools/filesystem/edit_impl.go` | Add `RemoveByString`, `RemoveByLines` types. Rewrite `RemoveOp` as embedded-pointer struct. Add `RemoveOp.JSONSchema()` and `Operation.JSONSchema()`. Delete `Op` alias, `OpSchema`, `JSONSchemaAlias`. Update `FileEditParams.Operations` to `[]Operation`. |
| `tools/filesystem/edit_test.go` | Add schema shape assertions: root `required`, outer `oneOf` on items, inner `oneOf` on `remove`. |

---

## Verification

`agentsdk tool schema file_edit` must show:

1. `required: [path, operations]` at the root.
2. `operations.items` is a `oneOf` with five branches; each has one key in
   `required` and `additionalProperties: false`.
3. The `remove` branch value is a `oneOf` with two branches: one requiring
   `old_string`, one requiring `lines`.
4. No `$defs`, `$schema`, or `$id` anywhere in the output.
5. Descriptions present on all inner fields.

`go test ./tools/filesystem/... ./tool/...` — runtime behaviour unchanged,
only the emitted schema changes.
