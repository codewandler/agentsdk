//go:build piper

package notify

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/amitybell/piper"
	voice "github.com/amitybell/piper-voice-jenny"
)

var (
	piperOnce sync.Once
	piperInst *piper.TTS
	piperErr  error
)

// initPiper lazily initializes the Piper TTS engine with the Jenny voice.
// On first call it extracts the embedded piper binary and voice model to
// $XDG_DATA_HOME/ab-piper. Subsequent calls return the cached instance.
func initPiper() (*piper.TTS, error) {
	piperOnce.Do(func() {
		piperInst, piperErr = piper.NewEmbedded("", voice.Asset)
	})
	return piperInst, piperErr
}

// piperSpeak synthesizes text with Piper (neural TTS) and starts background
// playback via aplay (ALSA) or paplay (PulseAudio). The function returns as
// soon as the WAV has been synthesized and queued for playback; it does NOT
// block until the audio has finished playing.
func piperSpeak(_ context.Context, text string) error {
	tts, err := initPiper()
	if err != nil {
		return fmt.Errorf("piper init: %w", err)
	}

	wav, err := tts.Synthesize(text)
	if err != nil {
		return fmt.Errorf("piper synthesize: %w", err)
	}

	// Write the WAV bytes to a temp file so the background player can consume it.
	tmp, err := os.CreateTemp("", "agentsdk-piper-*.wav")
	if err != nil {
		return fmt.Errorf("piper: create temp file: %w", err)
	}
	if _, err := tmp.Write(wav); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("piper: write wav: %w", err)
	}
	_ = tmp.Close()

	// Fire-and-forget: play the WAV in a goroutine detached from the tool
	// context so that context cancellation or timeouts do not interrupt playback.
	playWAVBackground(tmp.Name())
	return nil
}

