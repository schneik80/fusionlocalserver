# Linux tray app (Ayatana AppIndicator)

A tray / notifications-area icon to **start, stop, and monitor** the
`fusionlocalserver` binary. Written in Python + PyGObject as a
StatusNotifierItem, so it works on any desktop with SNI support — KDE, XFCE,
Budgie, Cinnamon natively, and GNOME via one extension.

The icon reflects server state and the menu offers **Start · Stop · Open in
browser · Show server log · Quit**; Start/Stop raise a desktop notification.

Two indicator backends, picked automatically at startup:

1. **`AyatanaAppIndicatorGlib` 2.0** (`libayatana-appindicator-glib`) — the
   current library. GTK-free: the menu is a `Gio.Menu` whose item actions live
   in a `Gio.SimpleActionGroup` under the (mandatory) `indicator.` namespace.
   Primary-clicking the icon opens the app in the browser where the panel
   supports it.
2. **`AyatanaAppIndicator3` 0.1** (`libayatana-appindicator-gtk3`) — the
   deprecated GTK3 library, kept as a fallback. Running on this backend prints
   *"libayatana-appindicator is deprecated. Please use
   libayatana-appindicator-glib"* — install the new package below and the app
   switches on next start.

## Dependencies (Fedora / Nobara)

```sh
sudo dnf install libayatana-appindicator-glib python3-gobject libnotify
```

(`libayatana-appindicator-glib` comes from the **terra** repo, enabled by
default on Nobara. Without it the app falls back to
`libayatana-appindicator-gtk3` + `gtk3`, with the deprecation warning.)

**GNOME only:** install the *AppIndicator and KStatusNotifierItem Support*
Shell extension so the icon appears in the top bar —
<https://extensions.gnome.org/extension/615/>. Other desktops show it natively.

## Run

```sh
/usr/bin/python3 fls-tray.py
```

Use the **system** interpreter — a pyenv/venv without `gi` will fail to import.

## Install / autostart

```sh
./install.sh              # add to the applications menu
./install.sh --autostart  # …and start it automatically at login
./install.sh --uninstall  # remove both
```

## How it manages the server

- **Start** launches `fusionlocalserver -tls` **detached** (its own session), so
  the server keeps running after you Quit the tray. The child pid is recorded in
  `~/.cache/fusionlocalserver/server.pid`.
- **Status** is read by probing the TCP port (the server has no health
  endpoint). The port comes from `~/.config/fusionlocalserver/server.json`
  (`port`), defaulting to `8080`. The icon is `network-transmit-receive` when up,
  `network-offline` when down. The probe completes a real TLS handshake
  (certificate unverified — liveness only) and closes cleanly, so polling
  doesn't fill the server log with `TLS handshake error … EOF` lines the way a
  bare connect-and-close would.
- **Stop** SIGTERMs the process group we started (graceful shutdown). It's only
  enabled for a server this app started — one started elsewhere shows as
  *Running (external)* and must be stopped where it was launched.
- **Open in browser** points at `https://localhost:<port>`; with `-tls` the cert
  is self-signed, so expect a one-time browser trust prompt.
- **Show server log** opens `~/.cache/fusionlocalserver/server.log`.

Build the binary first (`make build` at the repo root); the app finds it at the
repo root or on `$PATH`.

### Environment overrides

| Var | Meaning |
|-----|---------|
| `FLS_BINARY` | Path to the `fusionlocalserver` binary |
| `FLS_REPO`   | Repo root used as the server's working directory |
| `FLS_ARGS`   | Extra launch args appended after `-tls` |
