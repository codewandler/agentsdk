// Package notify provides the notify_send tool, which sends desktop
// notifications via the notify-send(1) utility (libnotify) and/or plays
// audio alerts via paplay/sox and text-to-speech via Piper (neural TTS,
// falls back to espeak). This works on any Linux desktop that implements
// the freedesktop.org Desktop Notifications specification (GNOME, KDE,
// sway, i3, hyprland, dunst, etc.).
package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/codewandler/core/tool"
)

const notifySendBin = "notify-send"

// NotifyParams are the parameters for the notify_send tool.
// At least one of Summary, Tone, or Speak must be provided.
type NotifyParams struct {
	// Summary is the notification title. Optional if Tone or Speak is set.
	Summary string `json:"summary,omitempty" jsonschema:"description=Notification title / summary. Optional when tone or speak is provided."`

	// Body is the optional notification body text.
	Body string `json:"body,omitempty" jsonschema:"description=Optional notification body text"`

	// Urgency sets the notification urgency level.
	// Valid values: low, normal, critical.
	Urgency string `json:"urgency,omitempty" jsonschema:"description=Urgency level: low / normal / critical"`

	// ExpireTime is the timeout in milliseconds after which the notification
	// is automatically dismissed. 0 means the server default.
	ExpireTime int `json:"expire_time,omitempty" jsonschema:"description=Auto-dismiss timeout in milliseconds (0 = server default),examples=[3000]"`

	// AppName overrides the application name shown in the notification.
	AppName string `json:"app_name,omitempty" jsonschema:"description=Application name shown in the notification (default: flai)"`

	// Icon is an icon name (e.g. dialog-information) or an absolute path to
	// an image file shown alongside the notification.
	Icon string `json:"icon,omitempty" jsonschema:"description=Icon name (e.g. dialog-information) or absolute path to an image file (default: dialog-information)"`

	// Category sets the notification category hint (e.g. im, email, transfer).
	Category string `json:"category,omitempty" jsonschema:"description=Notification category hint (e.g. im.received / email.arrived / transfer.complete)"`

	// Tone plays an audio alert using a named preset.
	// Valid values: beep, alarm, success, error, warning, info.
	// Uses paplay with freedesktop system sounds; falls back to sox synthesis.
	Tone string `json:"tone,omitempty" jsonschema:"description=Sound preset to play: beep / alarm / success / error / warning / info"`

	// Speak is text to be spoken aloud via Piper neural TTS (falls back to espeak).
	Speak string `json:"speak,omitempty" jsonschema:"description=Text to speak aloud via text-to-speech"`
}

var validUrgencies = map[string]bool{
	"low": true, "normal": true, "critical": true,
}

var validTones = map[string]bool{
	"beep": true, "alarm": true, "success": true,
	"error": true, "warning": true, "info": true,
}

// toneFiles maps a tone name to its freedesktop system sound file path.
var toneFiles = map[string]string{
	"beep":    "/usr/share/sounds/freedesktop/stereo/bell.oga",
	"alarm":   "/usr/share/sounds/freedesktop/stereo/alarm-clock-elapsed.oga",
	"success": "/usr/share/sounds/freedesktop/stereo/complete.oga",
	"error":   "/usr/share/sounds/freedesktop/stereo/dialog-error.oga",
	"warning": "/usr/share/sounds/freedesktop/stereo/dialog-warning.oga",
	"info":    "/usr/share/sounds/freedesktop/stereo/dialog-information.oga",
}

// soxToneArgs maps a tone name to its sox/play argument list for synthesis fallback.
var soxToneArgs = map[string][]string{
	"beep":    {"-n", "synth", "0.2", "sine", "880"},
	"alarm":   {"-n", "synth", "0.6", "sine", "880", "gain", "-n", "-3"},
	"success": {"-n", "synth", "0.3", "sine", "1047"},
	"error":   {"-n", "synth", "0.5", "sine", "220"},
	"warning": {"-n", "synth", "0.4", "sine", "660"},
	"info":    {"-n", "synth", "0.15", "sine", "523"},
}

// soxArgs returns the sox/play argument list for the given tone.
func soxArgs(tone string) []string {
	return soxToneArgs[tone]
}

// validateParams checks that the params are valid before executing.
func validateParams(p NotifyParams) error {
	if strings.TrimSpace(p.Summary) == "" && strings.TrimSpace(p.Tone) == "" && strings.TrimSpace(p.Speak) == "" {
		return fmt.Errorf("at least one of summary, tone, or speak must be provided")
	}
	if p.Urgency != "" && !validUrgencies[p.Urgency] {
		return fmt.Errorf("invalid urgency %q: must be low, normal, or critical", p.Urgency)
	}
	if p.Tone != "" && !validTones[p.Tone] {
		return fmt.Errorf("invalid tone %q: must be one of beep, alarm, success, error, warning, info", p.Tone)
	}
	return nil
}

// Tools returns the notify_send tool.
func Tools() []tool.Tool {
	return []tool.Tool{notifySend()}
}

func notifySend() tool.Tool {
	return tool.New("notify_send",
		"Send a desktop notification and/or audio alert. "+
			"Notifications use notify-send (libnotify) on any freedesktop.org-compliant desktop "+
			"(GNOME, KDE, sway, i3, hyprland, dunst, etc.). "+
			"Audio supports preset tones (beep, alarm, success, error, warning, info) via paplay or sox. "+
			"Text-to-speech is available via the speak field (uses Piper neural TTS, falls back to espeak). "+
			"Piper voice model is extracted to XDG_DATA_HOME on first use (~30MB, one-time cost). "+
			"Subsequent calls are fast. "+
			"Tone and speak can be used together. "+
			"Note: first call may take a few seconds while Piper initialises. "+
			"If Piper is unavailable, espeak is used as fallback. "+
			"Requires aplay or paplay for audio output. "+
			"TTS playback runs in the background — the tool returns immediately without waiting for audio to finish. "+
			"At least one of summary, tone, or speak must be provided.",
		func(ctx tool.Ctx, p NotifyParams) (tool.Result, error) {
			if err := validateParams(p); err != nil {
				return tool.Error(err.Error()), nil
			}

			execCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			var parts []string

			// 1. Desktop notification.
			if strings.TrimSpace(p.Summary) != "" {
				msg, err := runNotify(execCtx, p)
				if err != nil {
					return tool.Errorf("notify-send failed: %s", err), nil
				}
				parts = append(parts, msg)
			}

			// 2. Audio tone.
			if p.Tone != "" {
				if err := playTone(execCtx, p.Tone); err != nil {
					return tool.Errorf("audio tone failed: %s", err), nil
				}
				parts = append(parts, fmt.Sprintf("Tone played: %s", p.Tone))
			}

			// 3. Text-to-speech.
			if strings.TrimSpace(p.Speak) != "" {
				if err := speakMessage(execCtx, p.Speak); err != nil {
					return tool.Errorf("TTS failed: %s", err), nil
				}
				parts = append(parts, fmt.Sprintf("Speaking (background): %q", p.Speak))
			}

			return tool.Text(strings.Join(parts, "\n")), nil
		},
	)
}

// runNotify executes notify-send and returns a human-readable result string.
func runNotify(ctx context.Context, p NotifyParams) (string, error) {
	binPath, err := exec.LookPath(notifySendBin)
	if err != nil {
		return "", fmt.Errorf("notify-send not found: install libnotify (pacman -S libnotify or equivalent)")
	}

	args := buildArgs(p)

	start := time.Now()
	cmd := exec.CommandContext(ctx, binPath, args...)
	out, cmdErr := cmd.CombinedOutput()
	dur := time.Since(start)

	if cmdErr != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = cmdErr.Error()
		}
		return "", fmt.Errorf("%s (%s)", msg, dur.Round(time.Millisecond))
	}

	return fmt.Sprintf("Notification sent: %q (%s)", p.Summary, dur.Round(time.Millisecond)), nil
}

// playTone plays the named tone preset.
// It tries paplay with the freedesktop system sound first, then falls back to sox.
func playTone(ctx context.Context, tone string) error {
	// Try paplay with system sound file.
	if binPath, err := exec.LookPath("paplay"); err == nil {
		if file, ok := toneFiles[tone]; ok {
			cmd := exec.CommandContext(ctx, binPath, file)
			if out, err := cmd.CombinedOutput(); err == nil {
				return nil
			} else {
				_ = out // paplay failed; fall through to sox
			}
		}
	}

	// Fall back to sox.
	binPath, err := exec.LookPath("play")
	if err != nil {
		return fmt.Errorf("no audio player available (need paplay or sox/play)")
	}
	args := soxArgs(tone)
	cmd := exec.CommandContext(ctx, binPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// speakMessage speaks text using Piper (neural TTS) with espeak/spd-say as fallback.
// Audio playback is fire-and-forget: the function returns as soon as synthesis is
// complete (piper path) or immediately (espeak/spd-say path). The caller does NOT
// block until the audio has finished playing.
func speakMessage(ctx context.Context, text string) error {
	// Piper neural TTS — best quality. Synthesis is synchronous so errors are
	// surfaced; playback runs in the background via playWAVBackground.
	if err := piperSpeak(ctx, text); err == nil {
		return nil
	}

	// Fall back to espeak / spd-say — run entirely in a background goroutine
	// so the tool call returns immediately.
	for _, bin := range []string{"espeak", "spd-say"} {
		if binPath, err := exec.LookPath(bin); err == nil {
			go func(binPath, text string) {
				cmd := exec.Command(binPath, text)
				_ = cmd.Run()
			}(binPath, text)
			return nil
		}
	}
	return fmt.Errorf("no TTS available (need piper, espeak, or spd-say)")
}

// buildArgs assembles the notify-send argument list from the given params.
func buildArgs(p NotifyParams) []string {
	var args []string

	appName := p.AppName
	if appName == "" {
		appName = "flai"
	}
	args = append(args, "--app-name="+appName)

	if p.Urgency != "" {
		args = append(args, "--urgency="+p.Urgency)
	}
	if p.ExpireTime > 0 {
		args = append(args, fmt.Sprintf("--expire-time=%d", p.ExpireTime))
	}
	icon := p.Icon
	if icon == "" {
		icon = "dialog-information"
	}
	args = append(args, "--icon="+icon)
	if p.Category != "" {
		args = append(args, "--category="+p.Category)
	}

	// Positional arguments: summary [body]
	args = append(args, p.Summary)
	if p.Body != "" {
		args = append(args, p.Body)
	}

	return args
}
