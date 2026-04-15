package toolmgmt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/tool"
)

// ── StringSliceParam tests ─────────────────────────────────────────────────────

func TestToolMgmtStringSliceParam_Unmarshal_SingularString(t *testing.T) {
	var p struct {
		Tools tool.StringSliceParam `json:"tools"`
	}
	input := `{"tools": "file_*"}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file_*"}, []string(p.Tools))
}

func TestToolMgmtStringSliceParam_Unmarshal_ArrayOfStrings(t *testing.T) {
	var p struct {
		Tools tool.StringSliceParam `json:"tools"`
	}
	input := `{"tools": ["file_*", "bash"]}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file_*", "bash"}, []string(p.Tools))
}

func TestToolMgmtStringSliceParam_Unmarshal_EmptyArray(t *testing.T) {
	var p struct {
		Tools tool.StringSliceParam `json:"tools"`
	}
	input := `{"tools": []}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{}, []string(p.Tools))
}

func TestToolMgmtStringSliceParam_Unmarshal_Nil(t *testing.T) {
	var p struct {
		Tools tool.StringSliceParam `json:"tools"`
	}
	input := `{}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string(nil), []string(p.Tools))
}
