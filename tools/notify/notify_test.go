package notify_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/tools/notify"
)

func TestTools_ReturnsSingleTool(t *testing.T) {
	tools := notify.Tools()
	require.Len(t, tools, 1)
	require.Equal(t, "notify_send", tools[0].Name())
}

func TestTools_HasDescription(t *testing.T) {
	tools := notify.Tools()
	require.NotEmpty(t, tools[0].Description())
}

func TestTools_HasSchema(t *testing.T) {
	tools := notify.Tools()
	require.NotNil(t, tools[0].Schema())
}

func TestNotifyParams_SummaryIsOptional(t *testing.T) {
	// summary is no longer required; audio-only calls (tone or speak) are valid.
	tools := notify.Tools()
	schema := tools[0].Schema()
	require.NotNil(t, schema)

	for _, r := range schema.Required {
		require.NotEqual(t, "summary", r, "summary must NOT be a required schema field")
	}
}

func TestNotifyParams_SchemaHasToneAndSpeakFields(t *testing.T) {
	schema := notify.Tools()[0].Schema()
	require.NotNil(t, schema)

	b, err := json.Marshal(schema)
	require.NoError(t, err)

	schemaStr := string(b)
	require.Contains(t, schemaStr, `"tone"`, "schema should have a tone field")
	require.Contains(t, schemaStr, `"speak"`, "schema should have a speak field")
}
