# Sourced, hidden, by pi.tape: makes setup.sh's change to review, sets up pi
# with the nit skill on Gemini in Vertex AI, and has pi read the code first.
# Needs GOOGLE_CLOUD_PROJECT, and Application Default Credentials or
# GOOGLE_APPLICATION_CREDENTIALS, which it keeps across setup.sh's new HOME.
[ -n "$GOOGLE_CLOUD_PROJECT" ] || { echo 'set GOOGLE_CLOUD_PROJECT' >&2; return 1; }
export GOOGLE_APPLICATION_CREDENTIALS="${GOOGLE_APPLICATION_CREDENTIALS:-$HOME/.config/gcloud/application_default_credentials.json}"
export GOOGLE_CLOUD_LOCATION="${GOOGLE_CLOUD_LOCATION:-global}"
# A tmux server of the demo's own, apart from any the user runs.
export TMUX_TMPDIR=/tmp/nit-demo
tmux kill-server 2>/dev/null
source "$(dirname "${BASH_SOURCE[0]}")/setup.sh" || return
unset NIT_SEND # pi-send.ts sets it inside pi
# pi builds the package after its edit, so the packages it's likely to import
# compile now, before recording, instead of slowing it.
go build slices sort
export PI_OFFLINE=1
# Copies, not links, so pi shows no path outside the demo.
mkdir -p ~/.agents/skills ~/.pi/agent/extensions
cp -R "$(dirname "$demo")/skills/nit" ~/.agents/skills/
cp "$demo/pi-send.ts" "$demo/pi-demo.ts" ~/.pi/agent/extensions/
cat > ~/.pi/agent/settings.json <<'JSON'
{
  "defaultProvider": "google-vertex",
  "defaultModel": "gemini-3.5-flash",
  "defaultThinkingLevel": "low",
  "retry": { "baseDelayMs": 250 },
  "quietStartup": "header",
  "enableInstallTelemetry": false
}
JSON
cat > ~/.tmux.conf <<'TMUX'
set -g status off
set -g escape-time 0
set -g focus-events on
set -g extended-keys on
set -g extended-keys-format csi-u
set -g default-terminal tmux-256color
set -ga terminal-overrides ',xterm-256color:Tc'
set -g pane-border-style fg=colour238
set -g pane-active-border-style fg=colour238
# nit's diff gets the larger share of the width.
set-hook -g after-split-window 'resize-pane -x 60%'
TMUX
# pi reads the code and the change now, and pi-demo.ts hands what it read to
# the pi that's recorded.
pi -p --session-dir /tmp/nit-demo/prelude "understand the codebase and the change in progress" </dev/null >/dev/null || return
export NIT_DEMO_PRELUDE=$(ls /tmp/nit-demo/prelude/*.jsonl)
