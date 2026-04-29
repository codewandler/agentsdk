//go:build !piper

package notify

import (
	"context"
	"fmt"
)

// piperSpeak is a no-op stub when the piper build tag is not set.
// Build with -tags piper to embed the Piper neural TTS engine and Jenny voice
// model (~80 MB). Without the tag, TTS falls back to espeak/spd-say.
func piperSpeak(_ context.Context, _ string) error {
	return fmt.Errorf("piper TTS not available (build without -tags piper)")
}
