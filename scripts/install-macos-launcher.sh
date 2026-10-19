#!/bin/bash
# Bundle a private Ghostty runtime with its own macOS identity and icon.
# The installed gcrt executable is still used, independently of this runtime.
# An optional destination supports staging or installation outside ~/Applications.
set -euo pipefail
[[ "$(uname -s)" == Darwin ]] || { echo "This launcher requires macOS." >&2; exit 1; }
root="$(cd "$(dirname "$0")/.." && pwd)"
destination="${1:-$HOME/Applications/GCRT.app}"
[[ "$destination" == *.app ]] || { echo "Destination must end in .app" >&2; exit 1; }
[[ ! -e "$destination" ]] || { echo "Already exists; leaving it untouched: $destination" >&2; exit 1; }
ghostty="${GHOSTTY_APP:-/Applications/Ghostty.app}"
if [[ ! -d "$ghostty" && -z "${GHOSTTY_APP:-}" ]]; then ghostty="$HOME/Applications/Ghostty.app"; fi
[[ -x "$ghostty/Contents/MacOS/ghostty" ]] || { echo "Install Ghostty first, or set GHOSTTY_APP to its .app path." >&2; exit 1; }
work="$(mktemp -d "${TMPDIR:-/tmp}/gcrt-launcher.XXXXXX")"
app="$work/GCRT.app"
/usr/bin/ditto "$ghostty" "$app"
mkdir -p "$work/GCRT.iconset"
# Keep the runtime's nibs, frameworks and required metadata, but register only
# GCRT's identity. In particular, do not register duplicate Ghostty file handlers.
/usr/bin/python3 - "$app/Contents/Info.plist" "$root/packaging/macos/Info.plist" <<'PY'
import plistlib, sys
from pathlib import Path
path = Path(sys.argv[1])
info = plistlib.loads(path.read_bytes())
runtime_version = info['CFBundleShortVersionString']
info.update(plistlib.loads(Path(sys.argv[2]).read_bytes()))
info['GCRTRuntimeVersion'] = runtime_version
for key in ('CFBundleIconName', 'NSDockTilePlugIn', 'CFBundleDocumentTypes',
            'NSServices', 'UTExportedTypeDeclarations'):
    info.pop(key, None)
path.write_bytes(plistlib.dumps(info))
PY
/usr/bin/swiftc -module-cache-path "$work/swift-cache" \
  "$root/packaging/macos/gcrt-launcher.swift" -o "$app/Contents/MacOS/gcrt-launcher"
cp "$root/packaging/macos/start-session" "$app/Contents/Resources/"
chmod 755 "$app/Contents/MacOS/gcrt-launcher" "$app/Contents/Resources/start-session"

/usr/bin/swift -module-cache-path "$work/swift-cache" \
  "$root/packaging/macos/make-icon.swift" "$work/icon.png"
for size in 16 32 128 256 512; do
  /usr/bin/sips -z "$size" "$size" "$work/icon.png" --out "$work/GCRT.iconset/icon_${size}x${size}.png" >/dev/null
  /usr/bin/sips -z "$((size*2))" "$((size*2))" "$work/icon.png" --out "$work/GCRT.iconset/icon_${size}x${size}@2x.png" >/dev/null
done
/usr/bin/iconutil -c icns "$work/GCRT.iconset" -o "$app/Contents/Resources/GCRT.icns"
/usr/bin/plutil -lint "$app/Contents/Info.plist"
/bin/bash -n "$app/Contents/Resources/start-session"
# The copied bundle's original signature no longer covers its new identity.
# Ad-hoc sign this local build; never change the source Ghostty application.
/usr/bin/codesign --force --deep --sign - "$app"
/usr/bin/codesign --verify --deep --strict "$app"
mkdir -p "$(dirname "$destination")"
mv -n "$app" "$destination"
[[ ! -e "$app" ]] || { echo "Destination appeared during build; leaving it untouched." >&2; exit 1; }
echo "Installed: $destination"
echo "Open it from Finder or Spotlight, or drag it into the Dock."
