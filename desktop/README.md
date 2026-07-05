# desktop/ — OS-specific server manager apps

Small, native tray/menubar front-ends that **start, stop, and monitor** the
`fusionlocalserver` Go binary from the desktop, so you don't need a terminal to
run it. One subfolder per platform:

| Folder | Platform | Toolkit | Status |
|--------|----------|---------|--------|
| [`linux/`](linux/) | Linux tray (KDE/XFCE/… native, GNOME via extension) | libayatana-appindicator (PyGObject) | ✅ tray app (start/stop/status/notify) |
| `macos/`  | macOS menu bar | *(planned)* | — |
| `windows/`| Windows tray  | *(planned)* | — |

## How they talk to the server

These apps are thin managers around the existing binary — they add **no**
server code and open no ports of their own:

- **Start** launches the built `fusionlocalserver` binary detached (its own
  session), so the server keeps running if the manager window is closed.
- **Status** is read by probing the configured TCP port — the server has no
  health endpoint, and a listening socket is the ground truth. The port comes
  from `~/.config/fusionlocalserver/server.json` (`port`), defaulting to `8080`.
- **Stop** signals the process the manager started, tracked via a small pidfile
  so Stop still works after the manager app is restarted.

Building the binary is unchanged and lives at the repo root:

```sh
make build          # produces ./fusionlocalserver (UI embedded, client id baked in)
```

The managers locate that binary automatically (repo root, then `$PATH`); set
`FLS_BINARY` to point elsewhere.
