package server

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

func bookSourceID(bookSourceURL string) string {
	url := strings.TrimSpace(bookSourceURL)
	if url == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return "src_" + base64.RawURLEncoding.EncodeToString(sum[:12])
}

func sameBookSourceID(id string, bookSourceURL string) bool {
	return strings.TrimSpace(id) != "" && strings.TrimSpace(id) == bookSourceID(bookSourceURL)
}
