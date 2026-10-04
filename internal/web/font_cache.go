package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sync"
	"time"
)

type fontVersion struct {
	size    int64
	modTime time.Time
	digest  string
}

type fontVersionCache struct {
	mu      sync.Mutex
	entries map[string]fontVersion
}

func (c *fontVersionCache) digest(path string, info os.FileInfo) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[path]; ok && entry.size == info.Size() && entry.modTime.Equal(info.ModTime()) {
		return entry.digest
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return ""
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if c.entries == nil {
		c.entries = make(map[string]fontVersion)
	}
	c.entries[path] = fontVersion{size: info.Size(), modTime: info.ModTime(), digest: digest}
	return digest
}
