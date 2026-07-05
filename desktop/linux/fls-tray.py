#!/usr/bin/env python3
"""
Fusion Local Server — Linux tray app (libayatana-appindicator).

A StatusNotifierItem that lives in the tray / notifications area and lets you
start, stop, and monitor the `fusionlocalserver` Go binary. Part of desktop/
(OS-specific manager apps).

Behaviour:
  * The tray icon reflects server state (running / stopped) and the menu offers
    Start, Stop, Open in browser, Show log, and Quit.
  * The server is launched DETACHED (its own session) so it keeps running when
    the tray app quits. The process we started is recorded in a pidfile so Stop
    still works across app restarts.
  * "Running" is detected by probing the configured TCP port — the server has
    no health endpoint, so a listening socket is the ground truth. The port is
    read from ~/.config/fusionlocalserver/server.json (falling back to 8080).
  * Start/Stop raise a desktop notification.

Requires the SYSTEM PyGObject stack plus the AppIndicator + libnotify libs:
    gtk3, libayatana-appindicator-gtk3, python3-gobject, libnotify
On GNOME the icon needs the "AppIndicator and KStatusNotifierItem Support"
Shell extension (KDE/XFCE/Budgie/etc. render it natively). Run with the SYSTEM
interpreter:
    /usr/bin/python3 fls-tray.py

Environment overrides:
  FLS_BINARY  path to the fusionlocalserver binary (default: repo root, then $PATH)
  FLS_REPO    repo root used as the server's working directory
  FLS_ARGS    extra args appended to the launch command (default just adds -tls)
"""

from __future__ import annotations

import json
import os
import signal
import socket
import subprocess
import sys
from pathlib import Path
from shutil import which

try:
    import gi

    gi.require_version("Gtk", "3.0")
    gi.require_version("AyatanaAppIndicator3", "0.1")
    gi.require_version("Notify", "0.7")
    from gi.repository import (  # noqa: E402
        AyatanaAppIndicator3 as AppIndicator,
        Gio,
        GLib,
        Gtk,
        Notify,
    )
except (ImportError, ValueError) as exc:  # pragma: no cover - runtime env guard
    sys.stderr.write(
        "Fusion Local Server tray needs the system PyGObject + AppIndicator "
        "stack:\n  gtk3, libayatana-appindicator-gtk3, python3-gobject, "
        "libnotify\nRun it with the system interpreter, e.g.:\n\n"
        "    /usr/bin/python3 fls-tray.py\n\n"
        f"(import error: {exc})\n"
    )
    raise SystemExit(1)

APP_ID = "io.github.schneik80.FusionLocalServerTray"
APP_NAME = "Fusion Local Server"
DEFAULT_PORT = 8080
POLL_SECONDS = 2

# Themed icon names (present in Adwaita and most icon themes). The tray icon
# swaps between them to signal server state at a glance.
ICON_RUNNING = "network-transmit-receive"
ICON_STOPPED = "network-offline"


# ---- server discovery / state ------------------------------------------------


def repo_root() -> Path:
    env = os.environ.get("FLS_REPO")
    if env:
        return Path(env).expanduser()
    # desktop/linux/fls-tray.py -> repo root is two levels up.
    return Path(__file__).resolve().parents[2]


def server_binary() -> Path | None:
    env = os.environ.get("FLS_BINARY")
    if env:
        p = Path(env).expanduser()
        return p if p.exists() else None
    cand = repo_root() / "fusionlocalserver"
    if cand.exists():
        return cand
    found = which("fusionlocalserver")
    return Path(found) if found else None


def config_dir() -> Path:
    return Path(GLib.get_user_config_dir()) / "fusionlocalserver"


def state_dir() -> Path:
    d = Path(GLib.get_user_cache_dir()) / "fusionlocalserver"
    d.mkdir(parents=True, exist_ok=True)
    return d


def configured_port() -> int:
    """The server's bind port from server.json, or the 8080 default."""
    try:
        data = json.loads((config_dir() / "server.json").read_text())
        port = int(data.get("port") or 0)
        if 1 <= port <= 65535:
            return port
    except (OSError, ValueError, json.JSONDecodeError):
        pass
    return DEFAULT_PORT


def port_is_open(port: int) -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.4)
        try:
            return s.connect_ex(("127.0.0.1", port)) == 0
        except OSError:
            return False


class ServerManager:
    """Launches / signals the Go server and reports whether it is listening."""

    def __init__(self) -> None:
        self.pidfile = state_dir() / "server.pid"
        self.logfile = state_dir() / "server.log"

    def _read_pid(self) -> int | None:
        try:
            return int(self.pidfile.read_text().strip())
        except (OSError, ValueError):
            return None

    @staticmethod
    def _alive(pid: int) -> bool:
        try:
            os.kill(pid, 0)
        except OSError:
            return False
        return True

    def managed_pid(self) -> int | None:
        """The pid of a server WE started that is still alive, else None."""
        pid = self._read_pid()
        return pid if pid and self._alive(pid) else None

    def port(self) -> int:
        return configured_port()

    def is_running(self) -> bool:
        return port_is_open(self.port())

    def url(self) -> str:
        return f"https://localhost:{self.port()}"

    def start(self) -> None:
        binary = server_binary()
        if binary is None:
            raise FileNotFoundError(
                "fusionlocalserver binary not found — build it with `make build` "
                "(or set FLS_BINARY)."
            )
        args = [str(binary), "-tls"]
        if extra := os.environ.get("FLS_ARGS"):
            args += extra.split()
        stamp = GLib.DateTime.new_now_local().format_iso8601()
        logf = open(self.logfile, "ab", buffering=0)
        logf.write(f"\n=== started {stamp} ({' '.join(args)}) ===\n".encode())
        proc = subprocess.Popen(  # noqa: S603 - args are our own, not user input
            args,
            cwd=str(repo_root()),
            stdout=logf,
            stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL,
            start_new_session=True,  # detach: the server outlives the tray
        )
        self.pidfile.write_text(str(proc.pid))

    def stop(self) -> None:
        pid = self.managed_pid()
        if pid is None:
            self.pidfile.unlink(missing_ok=True)  # clear a stale file
            return
        try:
            os.killpg(os.getpgid(pid), signal.SIGTERM)
        except OSError:
            try:
                os.kill(pid, signal.SIGTERM)
            except OSError:
                pass
        self.pidfile.unlink(missing_ok=True)


# ---- tray UI -----------------------------------------------------------------


class TrayApp:
    def __init__(self) -> None:
        self.manager = ServerManager()

        self.menu = Gtk.Menu()
        self.status_item = Gtk.MenuItem(label="…")
        self.status_item.set_sensitive(False)
        self.start_item = Gtk.MenuItem(label="Start server")
        self.start_item.connect("activate", self.on_start)
        self.stop_item = Gtk.MenuItem(label="Stop server")
        self.stop_item.connect("activate", self.on_stop)
        self.open_item = Gtk.MenuItem(label="Open in browser")
        self.open_item.connect("activate", self.on_open)
        self.log_item = Gtk.MenuItem(label="Show server log")
        self.log_item.connect("activate", self.on_log)
        quit_item = Gtk.MenuItem(label="Quit")
        quit_item.connect("activate", self.on_quit)

        for widget in (
            self.status_item,
            Gtk.SeparatorMenuItem(),
            self.start_item,
            self.stop_item,
            self.open_item,
            self.log_item,
            Gtk.SeparatorMenuItem(),
            quit_item,
        ):
            self.menu.append(widget)
        self.menu.show_all()

        self.indicator = AppIndicator.Indicator.new(
            APP_ID, ICON_STOPPED, AppIndicator.IndicatorCategory.APPLICATION_STATUS
        )
        self.indicator.set_status(AppIndicator.IndicatorStatus.ACTIVE)
        self.indicator.set_title(APP_NAME)
        self.indicator.set_menu(self.menu)

        self.refresh()
        GLib.timeout_add_seconds(POLL_SECONDS, self._tick)

    def _tick(self) -> bool:
        self.refresh()
        return GLib.SOURCE_CONTINUE

    def refresh(self) -> None:
        running = self.manager.is_running()
        managed = self.manager.managed_pid() is not None
        have_binary = server_binary() is not None

        self.indicator.set_icon_full(
            ICON_RUNNING if running else ICON_STOPPED,
            "running" if running else "stopped",
        )
        if running:
            suffix = "" if managed else " (external)"
            self.status_item.set_label(f"Running{suffix} · {self.manager.url()}")
        elif have_binary:
            self.status_item.set_label("Stopped")
        else:
            self.status_item.set_label("Stopped · binary not found (make build)")

        self.start_item.set_sensitive(not running and have_binary)
        # We can only stop a process we started (tracked via the pidfile).
        self.stop_item.set_sensitive(running and managed)
        self.open_item.set_sensitive(running)

    def _notify(self, body: str, icon: str) -> None:
        Notify.Notification.new(APP_NAME, body, icon).show()

    def _refresh_soon(self) -> None:
        # Port state lags a start/stop by a beat; nudge one refresh, then the
        # periodic poll takes over.
        GLib.timeout_add(600, self._refresh_once)

    def _refresh_once(self) -> bool:
        self.refresh()
        return GLib.SOURCE_REMOVE

    # -- actions --
    def on_start(self, _item) -> None:
        try:
            self.manager.start()
            self._notify(f"Starting on {self.manager.url()}", ICON_RUNNING)
        except OSError as exc:
            self._notify(str(exc), ICON_STOPPED)
        self._refresh_soon()

    def on_stop(self, _item) -> None:
        self.manager.stop()
        self._notify("Stopping server…", ICON_STOPPED)
        self._refresh_soon()

    def on_open(self, _item) -> None:
        Gio.AppInfo.launch_default_for_uri(self.manager.url(), None)

    def on_log(self, _item) -> None:
        Gio.AppInfo.launch_default_for_uri(self.manager.logfile.as_uri(), None)

    def on_quit(self, _item) -> None:
        Gtk.main_quit()


def main() -> int:
    Notify.init(APP_NAME)
    TrayApp()
    # Let Ctrl-C / SIGTERM end the GLib loop cleanly.
    for sig in (signal.SIGINT, signal.SIGTERM):
        GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, sig, Gtk.main_quit)
    try:
        Gtk.main()
    finally:
        Notify.uninit()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
