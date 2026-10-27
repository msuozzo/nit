# Records pi.mp4: pi, a coding agent, opens a change in nit beside itself and
# acts on the comments sent to it. demo/keys.py runs it and draws the keys it
# sends onto pi-keys.mp4. Run from the repository root with Vertex AI set up as
# demo/pi-setup.sh describes:
#   uv run demo/keys.py

Output demo/pi.mp4

Require go
Require jj
Require jq
Require pi
Require tmux

Set Shell "bash"
Set FontSize 18
Set Width 1920
Set Height 1080
Set Padding 0
Set TypingSpeed 60ms
Set WaitTimeout 120s

# pi has read the code before recording starts.
Hide
Type "source demo/pi-setup.sh && tmux new-session pi" Enter
Wait+Screen /gemini/
Show
Sleep 1s

# pi opens the change in nit, in a pane beside its own, and replies. It's idle
# again when its editor's top border has no spinner.
Type "let's review this change in nit" Sleep 500ms Enter
Wait+Screen /NORMAL/
Wait+Screen /(?m)^─{20}[^\n]*\n[^\n]*\n─{20}/
Sleep 1s

# Find the line, comment on it, and send the comment to pi.
Type "/func Mean" Sleep 600ms Enter
Sleep 1s
Type "c" Sleep 600ms
Type "can we add a Median?" Sleep 800ms
Ctrl+S

# pi adds Median, and nit lists what changed until r reloads.
Wait+Screen /diff changed/
Sleep 1.5s
Type "r"
Sleep 2.5s

# Approve the whole change from the commit message at the top, and pi closes
# the nit pane, leaving its own pane the full width.
Type "gg" Sleep 600ms
Type "c" Sleep 500ms
Type "lgtm" Sleep 700ms
Ctrl+S
Wait+Screen /(?m)^─{100}/
Wait+Screen /(?m)^─{100}[^\n]*\n[^\n]*\n─{100}/
Sleep 2.5s

Hide
Ctrl+D
