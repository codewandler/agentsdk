package agentcontext

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func FragmentFingerprint(fragment ContextFragment) string {
	if fragment.Fingerprint != "" {
		return fragment.Fingerprint
	}
	var b strings.Builder
	b.WriteString(string(fragment.Key))
	b.WriteByte(0)
	b.WriteString(string(fragment.Role))
	b.WriteByte(0)
	b.WriteString(fragment.StartMarker)
	b.WriteByte(0)
	b.WriteString(fragment.EndMarker)
	b.WriteByte(0)
	b.WriteString(fragment.Content)
	b.WriteByte(0)
	b.WriteString(string(fragment.Authority))
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ProviderFingerprint(fragments []ContextFragment) string {
	var b strings.Builder
	for _, fragment := range fragments {
		b.WriteString(string(fragment.Key))
		b.WriteByte(0)
		b.WriteString(FragmentFingerprint(fragment))
		b.WriteByte(0)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}
