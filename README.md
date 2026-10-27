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

When the diff changes on disk, a notice in the bottom right lists the files
that changed until `r` reloads. nit checks every 2 s while it has focus, when
it regains focus, and on `check`.

nit picks light or dark to match the terminal's background, and shows the
diff side by side when its pane is 150 columns or wider. `--colorblind`
colors additions blue and deletions yellow, which red-green colorblind eyes
tell apart better than green and red.

Press `?` in nit for the keyboard shortcuts.

### Commit messages

With `-r` or `-f`/`-t`, each reviewed commit's description leads the diff as
an added file, `.review/COMMIT_MSG` (or `.review/COMMIT_MSG.<id>` per commit
when there are several), so prose gets comments like code. A real
`.review/COMMIT_MSG` in the working copy takes its place.

## Sending comments

Send delivers a comment straight to your coding agent, quoting the code it's
on, so the agent can act on it while you keep reviewing. The [agent
skill](#agent-skill) sets this up for agents like Claude Code. Send also
works where in-file comments can't: on removed lines, and on diffs that don't
end at the working copy.

Send runs `--send CMD`, or `NIT_SEND` from the environment, with `sh -c` for
each comment. The command gets `NIT_ROOT` set to the repository root, and the
comment on stdin as one line of JSON whose `prompt` field holds the whole
comment as a message. So any program can take comments, a log for instance:

```sh
NIT_SEND='jq -c . >> ~/review-comments.jsonl'
```

## Scripting nit

`nit --listen SOCK` lets another program take certain actions in nit. Each
connection to the unix socket sends one line of JSON and reads back `ok` or
an error. `{"op": "open", "dir": "/src/ws", "from": "main"}` shows another
diff, with keys named after the diff flags.
`{"op": "check"}` looks for changes on disk right away, and `{"op": "quit"}`
exits.

## Agent skill

`skills/nit` is an [Agent Skills](https://agentskills.io) skill that opens
nit beside your coding agent and acts on the comments you send from it. Link
it where your agent finds skills:

```
ln -s "$PWD/skills/nit" ~/.agents/skills/nit   # the shared location many agents read
ln -s "$PWD/skills/nit" ~/.claude/skills/nit   # Claude Code
```

Then ask your agent to review a change in nit. Outside tmux and Ghostty, it
prints the nit command to run in another terminal.

For most coding agents, you can configure `NIT_SEND` to deliver comments to
the agent's session. For Claude Code, no setup is needed as the skill
delivers comments through its undocumented messaging socket, so treat it as
experimental.

## Platforms

nit runs on Linux and macOS. The included agent skill knows how to control
tmux and Ghostty (macOS only) to open nit next to your agent automatically.

## Compatibility

While no code is shared between the two, nit maintains interoperability
with skepsis's data formats:

- viewed state and the saved light or dark choice, in
  `~/.local/share/skepsis/`
- `<review>` comment blocks written into the working copy
- the `--send` JSON payload and prompt, and `SKEPSIS_SEND` and `SKEPSIS_ROOT`
  beside `NIT_SEND` and `NIT_ROOT`

## License

[MIT](LICENSE)
