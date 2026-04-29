package notify

import (
	"os"
	"os/exec"
)

// playWAVBackground plays wavPath with aplay or paplay in a background goroutine.
// It removes wavPath when playback finishes (or immediately when no player is found).
// It is intentionally detached from any context so that the tool call can return
// without waiting for the audio to finish.
func playWAVBackground(wavPath string) {
	go func() {
		defer os.Remove(wavPath)
		for _, args := range [][]string{
			{"aplay", "-q", wavPath},
			{"paplay", wavPath},
		} {
			if binPath, err := exec.LookPath(args[0]); err == nil {
				cmd := exec.Command(binPath, args[1:]...)
				_ = cmd.Run()
				return
			}
		}
	}()
}
