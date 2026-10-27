# <sup><sub><samp>nit</samp></sub></sup>

nit is a terminal UI for reviewing jj and git diffs alongside a coding
agent. It's heavily inspired by, and compatible with,
[skepsis](https://github.com/oxidecomputer/skepsis), Oxide Computer's
browser-based review tool.

- **Pure Go.** One binary, [tree-sitter](https://tree-sitter.github.io)
  grammars included, with no cgo, browser, server, or Node.
- **Live agent loop.** Comments land in your agent's session as you send
  them, with diffs updated as they change.
- **Full-featured diffs.** Split or unified, with syntax highlighting,
  word-level changes, and a file tree.
- **Keyboard-first, mouse-friendly.** Navigation, search, and commenting all
  have familiar keybindings, but a click, scroll, or drag works just as well.

## Run

```
go install github.com/msuozzo/nit@latest   # the first build is slow, see https://go.dev/issue/75001

nit                       # jj: fork_point(trunk() | @) to @, git: merge base to the working tree
nit -r @                  # changes in revision(s)
nit -f main -t @          # changes between two revisions
nit --git -f HEAD         # git, from a commit to the working tree
nit -r @ src/             # only these paths
```

`--send CMD`, or `SKEPSIS_SEND` in the environment, makes Send pipe each
comment to a shell command instead. nit runs it with `sh -c` in the current
directory, with the comment as one JSON object on stdin and `SKEPSIS_ROOT`
set to the repository root.

nit picks light or dark to match the terminal's background, and shows the
diff side by side when its pane is 150 columns or wider.

Press `?` in nit for the keyboard shortcuts.

## Platforms

nit runs on Linux and macOS.

## Compatibility

While no code is shared between the two, nit maintains interoperability
with skepsis's data formats:

- viewed state and the saved light or dark choice, in
  `~/.local/share/skepsis/`
- `<review>` comment blocks written into the working copy
- the `--send` JSON payload and prompt

## License

[MIT](LICENSE)
