# Records demo.mp4: review a change in nit and send a comment to an agent,
# which demo/agent.sh stands in for. Run from the repository root:
#   vhs demo/demo.tape

Output demo/demo.mp4

Require go
Require jj
Require jq
Require perl

Set Shell "bash"
Set FontSize 18
Set Width 1280
Set Height 720
Set Padding 0
Set TypingSpeed 60ms
Set WaitTimeout 20s

Hide
Type "source demo/setup.sh && clear" Enter
Wait
Show

Type "nit -r @" Sleep 500ms Enter
Sleep 2.5s

# Find the line, comment on it, and send the comment to the agent.
Type "/tmp" Sleep 600ms Enter
Sleep 1.2s
Type "c" Sleep 600ms
Type "rename tmp to total" Sleep 800ms
Ctrl+S
Wait+Screen /sent to the session/
Sleep 1.5s

# The agent edits the file, and nit lists what changed until r reloads.
Wait+Screen /diff changed/
Sleep 2s
Type "r"
Sleep 2.5s

# Approve the whole change from the commit message at the top.
Type "gg" Sleep 600ms
Type "c" Sleep 500ms
Type "lgtm" Sleep 700ms
Ctrl+S
Sleep 2.5s
Type "q"
Sleep 500ms
