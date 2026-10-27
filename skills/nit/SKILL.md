---
name: nit
description: Review code changes with the user in nit, a terminal diff viewer opened beside this session, and act on the comments they send from it. Use when the user asks to review a change, a commit, or the working copy in nit, and whenever a message starting "Review comment on" arrives.
---

# Reviewing changes in nit

The user reads a diff in [nit](https://github.com/msuozzo/nit) beside this
session and comments on it. Each comment they send arrives here as a message
that quotes the lines it's on. You answer it or apply it, and nit lists the
files that changed until the user reloads with `r`.

`scripts/pane`, in this skill's base directory, runs the pane. It needs `nit`
on PATH and Python 3.

## Opening a diff

```sh
scripts/pane open -r @            # jj: the working-copy change
scripts/pane open -f main         # changes since main
scripts/pane open --git -f HEAD   # git: uncommitted changes
```

`open` takes nit's diff flags and paths. In tmux it splits this session's
pane, and in Ghostty on macOS it splits the focused terminal. Elsewhere it
prints the nit command for the user to run in another terminal. Once the pane
is up, `open` shows the next diff in the same pane.

Tell the user the pane is up: `c` comments on a line or a file header, `^s`
sends, `v` marks a file viewed, and `?` lists the rest.

## Handling comments

A sent comment looks like this:

````
Review comment on src/a.go, lines 42–45 (new side of `jj diff -r @`), in lines 40–47:

```
<the code>
```

why two?
````

A comment on a file header says "whole file" instead of a line span.

Read the intent before touching anything:

- **Edit request** ("let's", "can we", "rename this"): make the edit, run the
  narrowest check that covers it, reply in a line or two, and run
  `scripts/pane check` so nit shows what changed.
- **Question**: answer with what you checked, citing `file:line`. Don't edit.
- **LGTM**: the review is done. Say so, and run `scripts/pane quit` if
  nothing else is queued.

Keep replies short. The user is reading the diff, not the chat.

## Comments without Send

`scripts/pane` wires Send to this session through `NIT_SEND`, a command the
user set up to deliver to their agent, or else through Claude Code's
messaging socket, which Claude Code exposes to the commands it runs. Without
either, or when the user writes a comment into the code with `^o`, the
comment lands in the working copy as a `<review>` block:

```go
// <review>
// why two?
// </review>
```

When the user says they left comments, find them with
`grep -rn '<review>' .`, handle each one, and delete the block once it's
addressed.
