#!/bin/sh
# Regenerate the screenshots in docs/img.
#
# Everything runs against a throwaway demo config with invented hosts and a
# stand-in for ssh, so no real session ever reaches the repo. Needs tmux,
# `script`, and Python with Pillow.
#
#   sh scripts/screenshots.sh
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out="$root/docs/img"
work=$(mktemp -d)
sock=gcrt
mkdir -p "$out"

cleanup() {
  tmux -L "$sock" kill-server 2>/dev/null || true
  tmux -L "$sock-tabs" kill-server 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------- demo config
# $1 = directory, $2 = display mode, $3 = tree width
write_config() {
  mkdir -p "$1"
  cat > "$1/config.toml" <<EOF
[general]
default_transport    = "ssh"
display_mode         = "$2"
workspace_layout     = "sidebar"
workspace_tree_width = $3

[transports.ssh]
binary = "$work/fake-ssh"
EOF
}

cat > "$work/fake-ssh" <<'EOF'
#!/bin/sh
printf '\033[38;5;114mconnected\033[0m %s\n\n' "$*"
printf 'Linux edge-sw-04 6.1.0 #1 SMP\n'
printf 'Uptime:  42 days, 3 hours\n'
printf 'Load:    0.14, 0.09, 0.05\n\n'
printf 'A stand-in for ssh: gcrt is running this script, so the\n'
printf 'screenshots need no network and name no real hosts.\n'
sleep 86400
EOF
chmod +x "$work/fake-ssh"

# Invented hosts on documentation IPs (RFC 5737).
i=0
for spec in \
  "core-sw-01|network/switches|192.0.2.11" \
  "dist-sw-02|network/switches|192.0.2.12" \
  "edge-rtr-01|network/routers|192.0.2.21" \
  "bastion|network/routers|192.0.2.22" \
  "pve-01|servers/virt|192.0.2.31" \
  "k3s-01|servers/k8s|192.0.2.41" \
  "k3s-02|servers/k8s|192.0.2.42" \
  "app-01|servers/apps|192.0.2.51" \
  "build-01|servers/apps|192.0.2.52" \
  "nas-01|storage|192.0.2.61"
do
  i=$((i + 1))
  name=$(printf '%s' "$spec" | cut -d'|' -f1)
  group=$(printf '%s' "$spec" | cut -d'|' -f2)
  host=$(printf '%s' "$spec" | cut -d'|' -f3)
  printf '[[session]]\nid = "00000000-0000-4000-8000-%012d"\nname = "%s"\nslug = "%s"\ntransport = "ssh"\ngroup = "%s"\n\n  [session.ssh]\n  host = "%s"\n  user = "admin"\n\n' \
    "$i" "$name" "$name" "$group" "$host"
done > "$work/sessions.toml"

# ------------------------------------------------------------------- capture
# $1 = shot name, $2 = config dir, $3 = keys, $4 = window title.
# The pty is sized before gcrt starts, so the window is a predictable size
# rather than whatever the invoking terminal happens to be.
shot() {
  name=$1
  dir=$2
  keys=$3
  title=$4

  tmux -L "$sock" kill-server 2>/dev/null || true
  tmux -L "$sock-tabs" kill-server 2>/dev/null || true
  sleep 0.4

  # shellcheck disable=SC2059
  { printf '\033]11;rgb:1e1e/1e1e/1e1e\007'
    printf '\033[1;1R'
    sleep 4
    printf "$keys"
    sleep 4
  } | TERM=xterm-256color script -q /dev/null \
        sh -c "stty rows 27 cols 110; exec gcrt tui -config-dir '$dir'" \
        >/dev/null 2>&1 &
  runner=$!
  # Wait for the workspace to exist before touching it.
  tries=0
  while [ "$tries" -lt 40 ]; do
    tmux -L "$sock" list-panes -t gscrt/workspace:tree >/dev/null 2>&1 && break
    tries=$((tries + 1))
    sleep 0.5
  done

  # A predictable size, independent of whatever terminal invoked this.
  tmux -L "$sock" set-option -g window-size manual 2>/dev/null || true
  tmux -L "$sock" resize-window -t gscrt/workspace:tree -x 110 -y 26 2>/dev/null || true
  # Give the pane's program time to attach and redraw at the new size.
  sleep 3

  # The workspace is two panes side by side; capture-pane only ever yields one,
  # so take each and stitch them back into the window.
  panes=$(tmux -L "$sock" list-panes -t gscrt/workspace:tree -F '#{pane_id}')
  files=""
  for pane in $panes; do
    width=$(tmux -L "$sock" display-message -p -t "$pane" '#{pane_width}')
    # tmux trims trailing spaces, which would make each shot a different width;
    # pad each pane back to its own width so the columns line up.
    tmux -L "$sock" capture-pane -e -p -t "$pane" | python3 -c '
import re, sys
width = int(sys.argv[1])
sgr = re.compile("\x1b\\[[0-9;]*m")
for line in sys.stdin.read().split("\n"):
    pad = width - len(sgr.sub("", line))
    sys.stdout.write(line + " " * max(0, pad) + "\n")
' "$width" > "$work/pane-$name-$pane.txt"
    files="$files $work/pane-$name-$pane.txt"
  done
  # shellcheck disable=SC2086
  paste -d ' ' $files > "$work/$name.txt"

  python3 "$root/scripts/render.py" "$work/$name.txt" "$out/$name.png" --title "$title"

  # script(1) will not exit while the tmux client it started is attached, so
  # tear the workspace down before waiting on it.
  tmux -L "$sock" kill-server 2>/dev/null || true
  tmux -L "$sock-tabs" kill-server 2>/dev/null || true
  kill "$runner" 2>/dev/null || true
  wait "$runner" 2>/dev/null || true
  sleep 0.5
}

write_config "$work/wide" workspace 34
write_config "$work/roomy" workspace 52
cp "$work/sessions.toml" "$work/wide/sessions.toml"
cp "$work/sessions.toml" "$work/roomy/sessions.toml"

# 1. The sidebar: tree pinned, connections as tabs beside it.
shot sidebar "$work/wide" '\r\002tj\r\002t' 'gcrt — tree pinned, tabs beside it'

# 2. Right-click a session to get its menu.
shot menu "$work/wide" '\r\002t\033[<2;10;3M' 'gcrt — right-click a session'

# 3. The idle screen, before anything is open.
shot idle "$work/roomy" '' 'gcrt — nothing open yet'

# 4. Every key and command, from the menu bar's Help.
shot help "$work/roomy" '?' 'gcrt — keys and commands'

echo "wrote $(ls "$out"/*.png | wc -l | tr -d ' ') screenshots to $out"
