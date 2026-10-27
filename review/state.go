package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// stateDir holds skepsis's viewed state and settings, shared with it.
func stateDir() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "skepsis")
}

// Theme returns skepsis's saved theme: light, dark, or system.
func Theme() string {
	var s struct {
		Theme string `json:"theme"`
	}
	b, _ := os.ReadFile(filepath.Join(stateDir(), "settings.json"))
	if json.Unmarshal(b, &s) != nil || (s.Theme != "light" && s.Theme != "dark") {
		return "system"
	}
	return s.Theme
}

// Viewed state is one TSV file per repo of file, content hash, and the time
// it was marked in Unix ms. A file counts as viewed while its current hash
// is listed, so changing the file unviews it. Entries expire after 30 days.
const viewedTTL = 30 * 24 * time.Hour

type viewedEntry struct {
	file, hash string
	at         int64
}

func (r *Repo) viewedPath() string {
	sum := sha256.Sum256([]byte(r.Root))
	return filepath.Join(stateDir(), hex.EncodeToString(sum[:])[:12]+".viewed")
}

// loadViewed reads the unexpired entries. Lines without a time are expired.
func (r *Repo) loadViewed() []viewedEntry {
	b, _ := os.ReadFile(r.viewedPath())
	cutoff := time.Now().Add(-viewedTTL).UnixMilli()
	var es []viewedEntry
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		e := viewedEntry{file: f[0], hash: f[1]}
		if len(f) > 2 {
			e.at, _ = strconv.ParseInt(f[2], 10, 64)
		}
		if e.at >= cutoff {
			es = append(es, e)
		}
	}
	return es
}

// viewed returns the files whose current hash is marked viewed.
func (r *Repo) viewed(hashes map[string]string) map[string]string {
	r.viewedMu.Lock()
	defer r.viewedMu.Unlock()
	v := map[string]string{}
	for _, e := range r.loadViewed() {
		if hashes[e.file] == e.hash {
			v[e.file] = e.hash
		}
	}
	return v
}

// SetViewed marks or unmarks one version of a file as viewed.
func (r *Repo) SetViewed(file, hash string, viewed bool) error {
	r.viewedMu.Lock()
	defer r.viewedMu.Unlock()
	var b strings.Builder
	for _, e := range r.loadViewed() {
		if e.file != file || e.hash != hash {
			fmt.Fprintf(&b, "%s\t%s\t%d\n", e.file, e.hash, e.at)
		}
	}
	if viewed {
		fmt.Fprintf(&b, "%s\t%s\t%d\n", file, hash, time.Now().UnixMilli())
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.viewedPath(), []byte(b.String()), 0o644)
}
