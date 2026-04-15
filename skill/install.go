package skill

import (
	"context"
	"encoding/json"
)

// InstallAction describes what an Installer did (or would do).
type InstallAction string

const (
	InstallActionInstalled       InstallAction = "installed"
	InstallActionUpToDate        InstallAction = "up_to_date"
	InstallActionUpdateAvailable InstallAction = "update_available"
	InstallActionUpgraded        InstallAction = "upgraded"
)

// InstallResult describes the outcome of an Install call.
type InstallResult struct {
	Name       string
	SkillDir   string
	Action     InstallAction
	OldVersion string
	NewVersion string
	Diff       string
}

// Installer knows how to download and install a skill onto the local filesystem.
type Installer interface {
	Install(ctx context.Context, ref json.RawMessage, skillsDir string, upgrade bool) (*InstallResult, error)
	Name() string
}

// InstallerKey is the Extra() key for an Installer or InstallerDispatcher.
const InstallerKey = "skill_installer"
