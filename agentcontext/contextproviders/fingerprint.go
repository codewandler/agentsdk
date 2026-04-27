package contextproviders

import (
	"crypto/sha256"
	"encoding/hex"
)

func contentFingerprint(kind string, content string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
