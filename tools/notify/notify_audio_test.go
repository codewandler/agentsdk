package notify

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidTones_ContainsAllPresets(t *testing.T) {
	expected := []string{"beep", "alarm", "success", "error", "warning", "info"}
	for _, tone := range expected {
		require.True(t, validTones[tone], "expected tone %q to be in validTones", tone)
	}
}

func TestToneFiles_AllValidTonesHaveMapping(t *testing.T) {
	for tone := range validTones {
		_, ok := toneFiles[tone]
		require.True(t, ok, "tone %q has no entry in toneFiles", tone)
	}
}

func TestSoxToneArgs_AllValidTonesHaveArgs(t *testing.T) {
	for tone := range validTones {
		args := soxArgs(tone)
		require.NotEmpty(t, args, "tone %q has no sox args", tone)
	}
}

func TestValidate_EmptyParams_ReturnsError(t *testing.T) {
	err := validateParams(NotifyParams{})
	require.Error(t, err)
}

func TestValidate_SummaryOnly_OK(t *testing.T) {
	err := validateParams(NotifyParams{Summary: "hello"})
	require.NoError(t, err)
}

func TestValidate_ToneOnly_OK(t *testing.T) {
	err := validateParams(NotifyParams{Tone: "beep"})
	require.NoError(t, err)
}

func TestValidate_SpeakOnly_OK(t *testing.T) {
	err := validateParams(NotifyParams{Speak: "hello world"})
	require.NoError(t, err)
}

func TestValidate_InvalidTone_ReturnsError(t *testing.T) {
	err := validateParams(NotifyParams{Summary: "hi", Tone: "klaxon"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "klaxon")
}

func TestValidate_InvalidUrgency_ReturnsError(t *testing.T) {
	err := validateParams(NotifyParams{Summary: "hi", Urgency: "extreme"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "extreme")
}

// TestPlayWAVBackground_DeletesFileAfterPlayback verifies that playWAVBackground
// removes the temp file once playback ends (or immediately if no player exists).
func TestPlayWAVBackground_DeletesFileAfterPlayback(t *testing.T) {
	tmp, err := os.CreateTemp("", "test-notify-wav-*.wav")
	require.NoError(t, err)
	tmp.Close()

	playWAVBackground(tmp.Name())

	// The goroutine should finish quickly (no valid WAV / no audio player in CI)
	// and remove the file.
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(tmp.Name())
		return os.IsNotExist(statErr)
	}, 3*time.Second, 10*time.Millisecond, "playWAVBackground must remove the temp file after playback")
}

// TestSpeakMessage_ReturnsBeforePlaybackFinishes verifies that speakMessage
// does not block until audio has finished playing. With no TTS binaries
// available the function must still return promptly (goroutine, not inline).
func TestSpeakMessage_ReturnsBeforePlaybackFinishes(t *testing.T) {
	// We cannot guarantee piper/espeak is absent, but we can assert the call
	// completes well under the old 15-second timeout regardless of outcome.
	ctx := context.Background()

	start := time.Now()
	_ = speakMessage(ctx, "this is a short test phrase")
	elapsed := time.Since(start)

	// speakMessage should return in under 10 seconds even for long synthesis;
	// audio playback must NOT be blocking the caller.
	require.Less(t, elapsed, 10*time.Second,
		"speakMessage must return before playback finishes (elapsed: %s)", elapsed)
}
