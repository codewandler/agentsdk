// Package vision provides the vision tool for image understanding via a
// vision-capable LLM. The tool accepts image sources (URLs, file paths, or
// data URIs), sends them to a vision model, and returns the model's textual
// analysis.
//
// The tool uses an injected [unified.Client] for vision requests, independent
// of the agent's main conversation client. The plugin layer is responsible for
// constructing the client (typically OpenRouter with anthropic/claude-sonnet-4).
package vision

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/providerregistry"
	"github.com/codewandler/llmadapter/unified"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	// DefaultModel is the hardcoded vision model.
	DefaultModel = "anthropic/claude-sonnet-4"

	// maxFileSize is the maximum file size for local image reads (10 MB).
	maxFileSize = 10 * 1024 * 1024

	// maxOutputTokens caps the vision model response length.
	maxOutputTokens = 4096

	// defaultPrompt is used when no prompt is provided.
	defaultPrompt = "Describe this image in detail. Include any text, diagrams, UI elements, or notable visual features."
)

// ── Parameter types ───────────────────────────────────────────────────────────

// Action describes a single vision operation.
type Action struct {
	Action string   `json:"action" jsonschema:"description=Action to perform. Currently only 'understand'.,enum=understand,required"`
	Images []string `json:"images" jsonschema:"description=Image sources: HTTP(S) URLs or local file paths or data: URIs.,required"`
	Prompt string   `json:"prompt,omitempty" jsonschema:"description=What to analyze or describe. Defaults to a general description."`
}

// Params is the top-level input for the vision tool.
type Params struct {
	Actions []Action `json:"actions" jsonschema:"description=Vision actions to perform.,required"`
}

// ── Tools factory ─────────────────────────────────────────────────────────────

// Tools returns the vision tool slice. If client is nil the tool reports a
// configuration error at call time.
func Tools(client unified.Client) []tool.Tool {
	return []tool.Tool{visionTool(client)}
}

func visionTool(client unified.Client) tool.Tool {
	if client == nil {
		return tool.New("vision",
			"Analyze images using a vision model. This tool is unavailable until an OpenRouter API key is configured.",
			func(_ tool.Ctx, _ Params) (tool.Result, error) {
				return tool.Error("vision tool is not configured; set OPENROUTER_API_KEY or VISION_OPENROUTER_API_KEY"), nil
			},
		)
	}
	return tool.New("vision",
		"Analyze images using a vision model. Supports URLs, file paths, and data URIs. Send multiple images per action for comparison or context.",
		func(ctx tool.Ctx, p Params) (tool.Result, error) {
			return executeVision(ctx, client, p)
		},
		tool.WithGuidance[Params]("Use vision to understand screenshots, diagrams, photos, or any image content. "+
			"Provide a focused prompt for better results. Multiple images in one action are sent together for comparison."),
		visionIntent(),
	)
}

// ── Execution ─────────────────────────────────────────────────────────────────

func executeVision(ctx tool.Ctx, client unified.Client, p Params) (tool.Result, error) {
	if len(p.Actions) == 0 {
		return tool.Error("at least one action is required"), nil
	}

	rb := tool.NewResult()
	for i, action := range p.Actions {
		result, err := executeAction(ctx, client, action)
		if err != nil {
			return nil, fmt.Errorf("action[%d]: %w", i, err)
		}
		if len(p.Actions) > 1 {
			rb.Textf("## Action %d: %s", i+1, action.Action)
		}
		rb.Text(result)
	}
	return rb.Build(), nil
}

func executeAction(ctx tool.Ctx, client unified.Client, action Action) (string, error) {
	switch action.Action {
	case "understand":
		return executeUnderstand(ctx, client, action)
	default:
		return "", fmt.Errorf("unsupported action %q; supported: understand", action.Action)
	}
}

func executeUnderstand(ctx tool.Ctx, client unified.Client, action Action) (string, error) {
	if len(action.Images) == 0 {
		return "", fmt.Errorf("at least one image is required")
	}

	// Build content parts: images first, then prompt text.
	parts := make([]unified.ContentPart, 0, len(action.Images)+1)
	for i, img := range action.Images {
		imagePart, err := ResolveImage(ctx.WorkDir(), img)
		if err != nil {
			return "", fmt.Errorf("image[%d]: %w", i, err)
		}
		parts = append(parts, imagePart)
	}

	prompt := action.Prompt
	if prompt == "" {
		prompt = defaultPrompt
	}
	parts = append(parts, unified.TextPart{Text: prompt})

	maxTokens := maxOutputTokens
	req := unified.Request{
		Model:           DefaultModel,
		Stream:          false,
		MaxOutputTokens: &maxTokens,
		Messages: []unified.Message{{
			Role:    unified.RoleUser,
			Content: parts,
		}},
	}

	events, err := client.Request(ctx, req)
	if err != nil {
		return "", fmt.Errorf("vision request failed: %w", err)
	}

	resp, err := unified.Collect(ctx, events)
	if err != nil {
		return "", fmt.Errorf("vision response failed: %w", err)
	}

	return extractText(resp), nil
}

// ── Image source resolution ──────────────────────────────────────────────────

// ResolveImage converts an image string (URL, data URI, or file path) into a
// unified.ImagePart suitable for the vision model request.
func ResolveImage(workDir, image string) (unified.ImagePart, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return unified.ImagePart{}, fmt.Errorf("image source cannot be empty")
	}

	switch {
	case strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://"):
		return resolveURL(image), nil
	case strings.HasPrefix(image, "data:"):
		return resolveDataURI(image)
	default:
		return resolveFile(workDir, image)
	}
}

func resolveURL(url string) unified.ImagePart {
	return unified.ImagePart{
		Source: unified.BlobSource{
			Kind: unified.BlobSourceURL,
			URL:  url,
		},
	}
}

func resolveDataURI(dataURI string) (unified.ImagePart, error) {
	// Format: data:<mediatype>;base64,<data>
	rest := strings.TrimPrefix(dataURI, "data:")
	semicolonIdx := strings.Index(rest, ";")
	if semicolonIdx < 0 {
		return unified.ImagePart{}, fmt.Errorf("invalid data URI: missing semicolon")
	}
	mediaType := rest[:semicolonIdx]
	rest = rest[semicolonIdx+1:]

	if !strings.HasPrefix(rest, "base64,") {
		return unified.ImagePart{}, fmt.Errorf("invalid data URI: only base64 encoding is supported")
	}
	b64Data := strings.TrimPrefix(rest, "base64,")

	// Validate that the base64 data is decodable.
	if _, err := base64.StdEncoding.DecodeString(b64Data); err != nil {
		// Try RawStdEncoding (no padding).
		if _, err2 := base64.RawStdEncoding.DecodeString(b64Data); err2 != nil {
			return unified.ImagePart{}, fmt.Errorf("invalid data URI: bad base64: %w", err)
		}
	}

	return unified.ImagePart{
		Source: unified.BlobSource{
			Kind:     unified.BlobSourceBase64,
			Base64:   b64Data,
			MIMEType: mediaType,
		},
	}, nil
}

func resolveFile(workDir, path string) (unified.ImagePart, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(workDir, path)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return unified.ImagePart{}, fmt.Errorf("cannot access %q: %w", path, err)
	}
	if info.IsDir() {
		return unified.ImagePart{}, fmt.Errorf("%q is a directory, not an image file", path)
	}
	if info.Size() > maxFileSize {
		return unified.ImagePart{}, fmt.Errorf("file %q is %d bytes, exceeds maximum %d bytes (10 MB)", path, info.Size(), maxFileSize)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return unified.ImagePart{}, fmt.Errorf("read %q: %w", path, err)
	}

	mediaType := mimeFromExt(absPath)
	if mediaType == "" {
		return unified.ImagePart{}, fmt.Errorf("cannot determine image type for %q; use a standard extension (.png, .jpg, .gif, .webp, .svg)", path)
	}

	return unified.ImagePart{
		Source: unified.BlobSource{
			Kind:     unified.BlobSourceBase64,
			Base64:   base64.StdEncoding.EncodeToString(data),
			MIMEType: mediaType,
		},
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func extractText(resp unified.Response) string {
	var sb strings.Builder
	for _, part := range resp.Content {
		if tp, ok := part.(unified.TextPart); ok {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(tp.Text)
		}
	}
	if sb.Len() == 0 {
		return "(no text response from vision model)"
	}
	return sb.String()
}

func mimeFromExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	// Explicit map for common image types — mime.TypeByExtension may not
	// be available or correct on all platforms.
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	default:
		// Fall back to stdlib.
		mt := mime.TypeByExtension(ext)
		if strings.HasPrefix(mt, "image/") {
			return mt
		}
		return ""
	}
}

// ClientFromEnv creates a unified.Client for vision using environment
// variables. It checks VISION_OPENROUTER_API_KEY first, then falls back to
// OPENROUTER_API_KEY. Returns nil if no key is available.
func ClientFromEnv() unified.Client {
	apiKey := os.Getenv("VISION_OPENROUTER_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}
	if apiKey == "" {
		return nil
	}
	return ClientFromKey(apiKey)
}

// ClientFromKey creates a unified.Client for vision using the given
// OpenRouter API key. Returns nil on error.
func ClientFromKey(apiKey string) unified.Client {
	client, err := providerregistry.NewClient(providerregistry.ClientConfig{
		Type:   "openrouter_chat",
		APIKey: apiKey,
	})
	if err != nil {
		return nil
	}
	return client
}