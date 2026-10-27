// Package review runs the jj or git diff under review and keeps its review
// state: contents at the diff's endpoints, viewed files, <review> comments
// written into the working copy, and comments sent to a command. Files on
// disk and the --send payload use skepsis's formats, so the two interoperate.
package review

import (
	"bytes"
	"cmp"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Options selects the diff, with skepsis's flags.
type Options struct {
	Revisions string   // -r: changes in these revisions
	From, To  string   // -f, -t: changes between two revisions
	Git       bool     // use git even in a jj repo
	Paths     []string // limit the diff to these paths
	Send      string   // shell command that Send pipes comments to
}

// Repo is one diff under review.
type Repo struct {
	Root string
	VCS  string // jj or git

	dir     string   // where the diff runs, so relative paths resolve
	args    []string // diff arguments
	display []string // diff arguments as shown, if not args
	paths   []string
	ends    bool   // the diff has two endpoints to read contents from
	left    string // old endpoint
	right   string // new endpoint, "" for the working copy
	inline  bool   // the diff ends at the working copy
	revs    string // commits whose descriptions are reviewed, "" for none
	send    string

	msgMu    sync.Mutex
	msgs     map[string]string // message pseudo-files from the last Load
	viewedMu sync.Mutex
	sendMu   sync.Mutex
}

// Open finds the repository containing dir and the diff o selects.
func Open(dir string, o Options) (*Repo, error) {
	r := &Repo{dir: dir, paths: o.Paths, send: o.Send}
	if !o.Git {
		if root, err := output(dir, "jj", "root", "--ignore-working-copy"); err == nil {
			r.Root, r.VCS = root, "jj"
		}
	}
	if r.VCS == "" {
		root, err := output(dir, "git", "rev-parse", "--show-toplevel")
		if err != nil {
			return nil, fmt.Errorf("%s is not in a jj or git repository", dir)
		}
		r.Root, r.VCS = root, "git"
	}
	if r.VCS == "jj" {
		r.jjDiff(o)
		return r, nil
	}
	return r, r.gitDiff(o)
}

func (r *Repo) jjDiff(o Options) {
	if rev := o.Revisions; rev != "" && o.From == "" && o.To == "" {
		r.args, r.revs = []string{"-r", rev}, rev
		r.inline = rev == "@" || strings.HasSuffix(rev, "..@")
		if a, b, ok := splitRange(rev); ok {
			r.ends, r.left, r.right = true, a, b
		} else if !strings.Contains(rev, "..") {
			r.ends, r.left, r.right = true, rev+"-", rev
		}
	} else {
		from := o.From
		if from == "" && o.To == "" {
			from = "fork_point(trunk() | @)"
		}
		if from != "" {
			r.args = append(r.args, "--from", from)
		}
		if o.To != "" {
			r.args = append(r.args, "--to", o.To)
		}
		r.inline = o.To == "" || o.To == "@"
		r.ends, r.left, r.right = true, cmp.Or(from, "@"), o.To
		if o.From != "" {
			r.revs = "(" + o.From + ")..(" + cmp.Or(o.To, "@") + ")"
		}
	}
	if r.right == "@" {
		r.right = ""
	}
}

func (r *Repo) gitDiff(o Options) error {
	switch rev := o.Revisions; {
	case o.To != "":
		from := cmp.Or(o.From, "HEAD")
		r.args, r.ends, r.left, r.right, r.revs = []string{from, o.To}, true, from, o.To, from+".."+o.To
	case o.From != "":
		r.args, r.ends, r.left, r.inline, r.revs = []string{o.From}, true, o.From, true, o.From+"..HEAD"
	case rev != "":
		r.args, r.inline = []string{rev}, !strings.Contains(rev, "..")
		if a, b, ok := splitRange(rev); ok {
			r.ends, r.left, r.right, r.revs = true, a, b, rev
		} else if r.inline {
			r.ends, r.left, r.revs = true, rev, rev+"..HEAD"
		}
	default:
		for _, base := range []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"} {
			if mb, err := output(r.Root, "git", "merge-base", base, "HEAD"); err == nil {
				r.args, r.display = []string{mb}, []string{"--merge-base", base}
				r.ends, r.left, r.inline = true, mb, true
				return nil
			}
		}
		return errors.New("no base revision for the default diff (tried origin/HEAD, origin/main, origin/master, main, master), pass -r or -f/-t")
	}
	return nil
}

// splitRange splits A..B. It rejects A...B and one-sided ranges.
func splitRange(rev string) (a, b string, ok bool) {
	a, b, ok = strings.Cut(rev, "..")
	if !ok || a == "" || b == "" || strings.HasPrefix(b, ".") || strings.Contains(b, "..") {
		return "", "", false
	}
	return a, b, true
}

func (r *Repo) Name() string          { return filepath.Base(r.Root) }
func (r *Repo) CommentsEnabled() bool { return r.inline }
func (r *Repo) Expandable() bool      { return r.ends }
func (r *Repo) CanSend() bool         { return r.send != "" }

// Command is the diff command, quoted for a shell.
func (r *Repo) Command() string {
	args := r.args
	if r.display != nil {
		args = r.display
	}
	s := r.VCS + " diff"
	for _, a := range args {
		s += " " + shellQuote(a)
	}
	if len(r.paths) > 0 {
		s += " --"
		for _, p := range r.paths {
			s += " " + shellQuote(p)
		}
	}
	return s
}

var plainArg = regexp.MustCompile(`^[\w@%+=:,./^~-]+$`)

func shellQuote(s string) string {
	if plainArg.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Diff runs the diff and returns the patch in git format, with a/ and b/
// path prefixes whatever the user's config says. The descriptions of the
// commits under review lead it as added files, see messages.
func (r *Repo) Diff() (string, error) {
	patch, _, err := r.diff()
	return patch, err
}

func (r *Repo) diff() (patch string, msgs map[string]string, err error) {
	var args []string
	if r.VCS == "jj" {
		args = append([]string{"--color=never", "--no-pager", "--config", "diff.git.show-path-prefix=true", "diff", "--git"}, r.args...)
	} else {
		args = append([]string{"-c", "diff.mnemonicPrefix=false", "-c", "diff.noprefix=false",
			"-c", "diff.srcPrefix=a/", "-c", "diff.dstPrefix=b/",
			"diff", "--no-color", "--no-ext-diff", "--no-relative"}, r.args...)
	}
	if len(r.paths) > 0 {
		args = append(append(args, "--"), r.paths...)
	}
	out, err := run(r.dir, r.VCS, args...)
	if err != nil {
		return "", nil, fmt.Errorf("%s diff: %w", r.VCS, err)
	}
	patch = string(out)
	msgs = r.messages()
	var head strings.Builder
	for _, name := range slices.Sorted(maps.Keys(msgs)) {
		if strings.Contains("\n"+patch, "\ndiff --git a/"+name+" b/"+name+"\n") {
			delete(msgs, name) // the working copy has a real one
			continue
		}
		lines := strings.Split(strings.TrimRight(msgs[name], "\n"), "\n")
		fmt.Fprintf(&head, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", name, name, name, len(lines))
		for _, l := range lines {
			head.WriteString("+" + l + "\n")
		}
	}
	return head.String() + patch, msgs, nil
}

// MessagePath is the pseudo-file a reviewed commit's description shows as,
// with a .<id> suffix per commit when there are several.
const MessagePath = ".review/COMMIT_MSG"

// messages reads the descriptions of the commits under review, keyed by
// pseudo-file path. Commits without a description are skipped.
func (r *Repo) messages() map[string]string {
	if r.revs == "" {
		return nil
	}
	var out []byte
	var err error
	if r.VCS == "jj" {
		out, err = run(r.Root, "jj", "--ignore-working-copy", "--color=never", "--no-pager", "log", "--no-graph",
			"-r", r.revs, "-T", `change_id.short(8) ++ "\0" ++ description ++ "\0"`)
	} else {
		out, err = run(r.Root, "git", "log", "--format=%h%x00%B%x00", r.revs)
	}
	if err != nil {
		return nil
	}
	var ids, texts []string
	f := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(f); i += 2 {
		if strings.TrimSpace(f[i+1]) != "" {
			ids, texts = append(ids, strings.TrimSpace(f[i])), append(texts, f[i+1])
		}
	}
	msgs := map[string]string{}
	for i, id := range ids {
		if len(ids) == 1 {
			msgs[MessagePath] = texts[i]
		} else {
			msgs[MessagePath+"."+id] = texts[i]
		}
	}
	return msgs
}

// Snapshot is one run of the diff with the state shown alongside it.
type Snapshot struct {
	Patch      string
	PatchHash  string
	FileHashes map[string]string // file → content hash, the viewed-state key
	Viewed     map[string]string // viewed files → their hash
	Syntaxes   map[string]*Syntax
	Pseudo     map[string]bool // message pseudo-files, not in the working copy
}

func (r *Repo) Load() (*Snapshot, error) {
	patch, msgs, err := r.diff()
	if err != nil {
		return nil, err
	}
	r.msgMu.Lock()
	r.msgs = msgs
	r.msgMu.Unlock()
	s := &Snapshot{Patch: patch, PatchHash: hashOf(patch), FileHashes: FileHashes(patch), Syntaxes: map[string]*Syntax{}, Pseudo: map[string]bool{}}
	for name := range msgs {
		s.Pseudo[name] = true
	}
	s.Viewed = r.viewed(s.FileHashes)
	for f := range s.FileHashes {
		s.Syntaxes[f] = r.syntax(f)
	}
	return s, nil
}

// PatchHash runs the diff and hashes it, to notice changes.
func (r *Repo) PatchHash() (string, error) {
	patch, err := r.Diff()
	return hashOf(patch), err
}

func hashOf(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

var diffGit = regexp.MustCompile(`^diff --git a/.+ b/(.+)$`)

// FileHashes keys each file of a patch by its b/ path. The hash is the new
// blob id from the index line. Pure renames and mode changes have none and
// hash their header lines instead.
func FileHashes(patch string) map[string]string {
	hashes := map[string]string{}
	var file string
	var header []string
	flush := func() {
		if file != "" {
			hashes[file] = hashOf(strings.Join(header, "\n"))
		}
	}
	for _, l := range strings.Split(patch, "\n") {
		if m := diffGit.FindStringSubmatch(l); m != nil {
			flush()
			file, header = m[1], []string{l}
		} else if file == "" {
			continue
		} else if ids, ok := strings.CutPrefix(l, "index "); ok {
			ids, _, _ = strings.Cut(ids, " ")
			if _, b, ok := strings.Cut(ids, ".."); ok {
				hashes[file] = b
			}
			file = ""
		} else {
			header = append(header, l)
		}
	}
	flush()
	return hashes
}

// Contents returns a file at both endpoints of the diff. A side is nil where
// the file doesn't exist.
func (r *Repo) Contents(path, oldPath string) (old, new *string, err error) {
	r.msgMu.Lock()
	msg, ok := r.msgs[path]
	r.msgMu.Unlock()
	if ok {
		return nil, &msg, nil
	}
	if !r.ends {
		return nil, nil, errors.New("this diff has no two endpoints to expand from")
	}
	old = r.show(r.left, cmp.Or(oldPath, path))
	if r.right != "" {
		return old, r.show(r.right, path), nil
	}
	return old, workingCopy(filepath.Join(r.Root, path)), nil
}

// show reads a file at a revision, nil if it isn't there.
func (r *Repo) show(rev, path string) *string {
	var out []byte
	var err error
	if r.VCS == "jj" {
		out, err = run(r.Root, "jj", "--ignore-working-copy", "--color=never", "--no-pager",
			"file", "show", "-r", rev, "--", `root-file:`+fileset(path))
	} else {
		out, err = run(r.Root, "git", "cat-file", "blob", rev+":"+path)
	}
	if err != nil {
		return nil
	}
	return ptr(string(out))
}

// workingCopy reads a file as the diff shows it: a symlink is its target
// path. Missing files are nil.
func workingCopy(abs string) *string {
	if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		t, err := os.Readlink(abs)
		if err != nil {
			return nil
		}
		return &t
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	return ptr(string(b))
}

func ptr[T any](v T) *T { return &v }

// fileset quotes a path as a jj fileset string literal.
func fileset(p string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(p) + `"`
}

// run executes a command and returns its stdout, or its stderr as the error.
func run(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, errors.New(msg)
		}
		return nil, err
	}
	return out, nil
}

func output(dir, name string, args ...string) (string, error) {
	out, err := run(dir, name, args...)
	return strings.TrimSpace(string(out)), err
}
