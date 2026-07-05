#!/usr/bin/env bash
# Install (or remove) the "Fusion Local Server (tray)" launcher. By default it
# lands in the applications menu; pass --autostart to also start it at login.
# The .desktop is generated from the .in template with an absolute Exec that
# runs the app under the SYSTEM python (so a pyenv/venv can't shadow PyGObject).
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
app_id="io.github.schneik80.FusionLocalServerTray"
apps="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
autostart="${XDG_CONFIG_HOME:-$HOME/.config}/autostart"

if [[ "${1:-}" == "--uninstall" || "${1:-}" == "-u" ]]; then
  rm -f "$apps/$app_id.desktop" "$autostart/$app_id.desktop"
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" || true
  echo "Removed launcher and autostart entry."
  exit 0
fi

script="$here/fls-tray.py"
chmod +x "$script"
mkdir -p "$apps"
sed "s|@EXEC@|/usr/bin/python3 $script|" "$here/$app_id.desktop.in" > "$apps/$app_id.desktop"
command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$apps" || true
echo "Installed $apps/$app_id.desktop"

if [[ "${1:-}" == "--autostart" || "${1:-}" == "-a" ]]; then
  mkdir -p "$autostart"
  cp "$apps/$app_id.desktop" "$autostart/$app_id.desktop"
  echo "Enabled autostart at login ($autostart/$app_id.desktop)."
fi

echo
echo "GNOME users: the tray icon needs the 'AppIndicator and KStatusNotifierItem"
echo "Support' extension — https://extensions.gnome.org/extension/615/"
echo "Uninstall with: $0 --uninstall"
