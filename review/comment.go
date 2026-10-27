package review

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Syntax is a line-comment syntax. Review comments are written as
//
//	<indent><prefix> <review> <suffix>
//	<indent><prefix> text <suffix>
//	<indent><prefix> </review> <suffix>
//
// after the line they discuss, and resolved by deleting the block.
type Syntax struct {
	Prefix, Suffix string
}

var (
	hashC    = Syntax{Prefix: "#"}
	slashC   = Syntax{Prefix: "//"}
	xmlC     = Syntax{Prefix: "<!--", Suffix: "-->"}
	blockC   = Syntax{Prefix: "/*", Suffix: "*/"}
	dashC    = Syntax{Prefix: "--"}
	percentC = Syntax{Prefix: "%"}
	semiC    = Syntax{Prefix: ";"}
	quoteC   = Syntax{Prefix: `"`}
)

var byExt = map[string]Syntax{}
var byName = map[string]Syntax{}

func init() {
	for s, exts := range map[Syntax]string{
		hashC: ".sh .bash .zsh .fish .ksh .py .pyi .pyw .rb .rake .gemspec .ru .pl .pm .yaml .yml .toml " +
			".r .ex .exs .nix .nu .tf .tfvars .hcl .conf .mk .cmake .ps1 .jl .coffee .tcl .bzl .bazel .star " +
			".txtpb .textproto .pbtxt .gn .gni .awk .properties .env .dockerfile .gitignore .gitattributes",
		slashC: ".go .rs .c .h .cc .cpp .cxx .hpp .hh .hxx .m .mm .cs .java .kt .kts .swift .scala .sc .dart " +
			".php .zig .js .mjs .cjs .jsx .ts .mts .cts .tsx .json .json5 .jsonc .proto .groovy .gradle .vue " +
			".sol .d .v .glsl .wgsl .hlsl .thrift .jsonnet .libsonnet",
		xmlC:     ".html .htm .xhtml .xml .svg .md .markdown .mdx .gohtml .xsl .plist",
		blockC:   ".css .scss .sass .less",
		dashC:    ".sql .lua .hs .lhs .elm .purs",
		percentC: ".tex .sty .cls .bib .erl .hrl",
		semiC:    ".ini .el .lisp .lsp .cl .clj .cljs .cljc .edn .scm .ss .rkt .asm",
		quoteC:   ".vim",
	} {
		for _, e := range strings.Fields(exts) {
			byExt[e] = s
		}
	}
	for s, names := range map[Syntax]string{
		hashC: "makefile gnumakefile dockerfile containerfile gemfile rakefile podfile brewfile vagrantfile " +
			"build workspace codeowners .bashrc .zshrc .profile .bash_profile .gitconfig .editorconfig requirements.txt",
		quoteC: ".vimrc .gvimrc",
	} {
		for _, n := range strings.Fields(names) {
			byName[n] = s
		}
	}
}

var byInterp = map[string]Syntax{
	"sh": hashC, "bash": hashC, "zsh": hashC, "fish": hashC, "dash": hashC, "ksh": hashC,
	"python": hashC, "python3": hashC, "ruby": hashC, "perl": hashC, "Rscript": hashC,
	"node": slashC, "deno": slashC, "bun": slashC, "lua": dashC,
}

// SyntaxFor returns a file's comment syntax from its name, or from the
// shebang on its first line. ok is false for unknown file types, which take
// bare markers.
func SyntaxFor(path, firstLine string) (s Syntax, ok bool) {
	base := strings.ToLower(filepath.Base(path))
	if s, ok := byName[base]; ok {
		return s, true
	}
	if strings.HasPrefix(base, "dockerfile.") {
		return hashC, true
	}
	if s, ok := byExt[filepath.Ext(base)]; ok {
		return s, true
	}
	if interp, ok := strings.CutPrefix(firstLine, "#!"); ok {
		f := strings.Fields(interp)
		if len(f) > 0 && filepath.Base(f[0]) == "env" {
			f = f[1:]
			for len(f) > 0 && strings.HasPrefix(f[0], "-") {
				f = f[1:]
			}
		}
		if len(f) > 0 {
			name := strings.TrimRight(filepath.Base(f[0]), "0123456789.")
			s, ok := byInterp[name]
			return s, ok
		}
	}
	return Syntax{}, false
}

// syntax looks up a file in the working copy, nil if unknown.
func (r *Repo) syntax(path string) *Syntax {
	if s, ok := SyntaxFor(path, ""); ok {
		return &s
	}
	if f, err := os.Open(filepath.Join(r.Root, path)); err == nil {
		defer f.Close()
		line, _ := bufio.NewReader(f).ReadString('\n')
		if s, ok := SyntaxFor(path, strings.TrimSpace(line)); ok {
			return &s
		}
	}
	return nil
}

// Tag matches a line holding just the marker tag, such as "<review>".
func (s Syntax) Tag(tag string) *regexp.Regexp {
	suffix := ""
	if s.Suffix != "" {
		suffix = `(?:\s*` + regexp.QuoteMeta(s.Suffix) + `)?`
	}
	return regexp.MustCompile(`^\s*` + regexp.QuoteMeta(s.Prefix) + `\s*` + regexp.QuoteMeta(tag) + suffix + `\s*$`)
}

// Strip returns the text of a comment line inside a block.
func (s Syntax) Strip(line string) string {
	t := strings.TrimPrefix(strings.TrimLeft(line, " \t"), s.Prefix)
	if s.Suffix != "" {
		t = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(t, " \t"), s.Suffix), " ")
	}
	return strings.TrimPrefix(t, " ")
}

func (s Syntax) line(indent, body string) string {
	parts := slices.DeleteFunc([]string{s.Prefix, body, s.Suffix}, func(p string) bool { return p == "" })
	return indent + strings.Join(parts, " ")
}

// editFile rewrites a working-copy file line by line. It refuses symlinks,
// which would edit the link's target instead.
func (r *Repo) editFile(file string, edit func(lines []string, syn Syntax) ([]string, error)) error {
	abs := filepath.Join(r.Root, file)
	fi, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("can't comment on symlink %s", file)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	syn, _ := SyntaxFor(file, lines[0])
	if lines, err = edit(lines, syn); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(strings.Join(lines, "\n")), fi.Mode().Perm())
}

// AddComment writes a <review> block into a file after line afterLine.
// lineText is the line as the diff showed it, so a file that changed since
// can't take the comment on the wrong line.
func (r *Repo) AddComment(file string, afterLine int, lineText, text string) error {
	return r.editFile(file, func(lines []string, syn Syntax) ([]string, error) {
		if afterLine < 1 || afterLine > len(lines) {
			return nil, fmt.Errorf("line %d is out of range (file has %d lines)", afterLine, len(lines))
		}
		target := lines[afterLine-1]
		if strings.TrimSuffix(target, "\r") != strings.TrimSuffix(lineText, "\r") {
			return nil, fmt.Errorf("line %d changed on disk, refresh and try again", afterLine)
		}
		indent := target[:len(target)-len(strings.TrimLeft(target, " \t"))]
		eol := ""
		if strings.HasSuffix(target, "\r") {
			eol = "\r"
		}
		block := []string{syn.line(indent, "<review>") + eol}
		for _, t := range strings.Split(text, "\n") {
			block = append(block, syn.line(indent, t)+eol)
		}
		block = append(block, syn.line(indent, "</review>")+eol)
		return append(lines[:afterLine], append(block, lines[afterLine:]...)...), nil
	})
}

// RemoveComment deletes the <review> block whose open tag is on line.
func (r *Repo) RemoveComment(file string, line int) error {
	return r.editFile(file, func(lines []string, syn Syntax) ([]string, error) {
		if line < 1 || line > len(lines) || !syn.Tag("<review>").MatchString(lines[line-1]) {
			return nil, fmt.Errorf("line %d is not a <review> open tag", line)
		}
		end := line
		for closeRe := syn.Tag("</review>"); end < len(lines) && !closeRe.MatchString(lines[end]); end++ {
		}
		if end == len(lines) {
			return nil, fmt.Errorf("no </review> after line %d", line)
		}
		return append(lines[:line-1], lines[end+1:]...), nil
	})
}
