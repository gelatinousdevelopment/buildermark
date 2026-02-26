#!/usr/bin/env python3

import os
import signal
import subprocess
import threading
import time
import webbrowser

import gi

gi.require_version("Gtk", "3.0")
from gi.repository import GLib, Gtk

APP_ID = "dev.buildermark.LocalTray"
DEFAULT_URL = "http://localhost:7022"


class BuildermarkTrayApp:
    def __init__(self) -> None:
        self.binary_path = os.environ.get("BUILDERMARK_BINARY", "/app/bin/buildermark-local")
        self.server_url = os.environ.get("BUILDERMARK_LOCAL_URL", DEFAULT_URL)

        data_home = os.environ.get("XDG_DATA_HOME", os.path.expanduser("~/.local/share"))
        db_dir = os.path.join(data_home, "buildermark")
        os.makedirs(db_dir, exist_ok=True)
        self.db_path = os.environ.get("BUILDERMARK_LOCAL_DB_PATH", os.path.join(db_dir, "local.db"))

        self.server_proc: subprocess.Popen[str] | None = None

        self.status_icon = Gtk.StatusIcon.new_from_icon_name("network-server")
        self.status_icon.set_tooltip_text("Buildermark Local")
        self.status_icon.set_visible(True)
        self.status_icon.connect("popup-menu", self.on_popup_menu)
        self.status_icon.connect("activate", self.on_open_clicked)

        self.menu = Gtk.Menu()

        self.status_item = Gtk.MenuItem(label="Status: Starting…")
        self.status_item.set_sensitive(False)
        self.menu.append(self.status_item)

        self.open_item = Gtk.MenuItem(label="Open Buildermark Local")
        self.open_item.connect("activate", self.on_open_clicked)
        self.menu.append(self.open_item)

        self.menu.append(Gtk.SeparatorMenuItem())

        self.settings_item = Gtk.MenuItem(label="Settings")
        self.settings_item.connect("activate", self.on_settings_clicked)
        self.menu.append(self.settings_item)

        self.quit_item = Gtk.MenuItem(label="Quit")
        self.quit_item.connect("activate", self.on_quit_clicked)
        self.menu.append(self.quit_item)

        self.menu.show_all()

        self.settings_window: Gtk.Window | None = None

        self.start_server()
        GLib.timeout_add_seconds(2, self.update_status)

    def start_server(self) -> None:
        if self.server_proc is not None and self.server_proc.poll() is None:
            return

        if not os.path.exists(self.binary_path):
            self.status_item.set_label(f"Status: Binary missing ({self.binary_path})")
            return

        env = os.environ.copy()
        env["BUILDERMARK_LOCAL_DB_PATH"] = self.db_path

        self.server_proc = subprocess.Popen(
            [self.binary_path, "-addr", ":7022", "-db", self.db_path],
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            text=True,
        )

        threading.Thread(target=self.wait_for_server_exit, daemon=True).start()

    def wait_for_server_exit(self) -> None:
        if self.server_proc is None:
            return
        self.server_proc.wait()
        GLib.idle_add(self.update_status)

    def update_status(self) -> bool:
        if self.server_proc is None:
            self.status_item.set_label("Status: Not running")
            return True

        code = self.server_proc.poll()
        if code is None:
            self.status_item.set_label("Status: Running")
        else:
            self.status_item.set_label(f"Status: Stopped (exit {code})")

        return True

    def on_popup_menu(self, _icon: Gtk.StatusIcon, button: int, activate_time: int) -> None:
        self.menu.popup(None, None, Gtk.StatusIcon.position_menu, self.status_icon, button, activate_time)

    def on_open_clicked(self, *_args) -> None:
        webbrowser.open(self.server_url)

    def on_settings_clicked(self, *_args) -> None:
        if self.settings_window is None:
            self.settings_window = Gtk.Window(title="Buildermark Local Settings")
            self.settings_window.set_default_size(420, 120)
            self.settings_window.set_skip_taskbar_hint(True)
            self.settings_window.set_keep_above(True)
            self.settings_window.connect("delete-event", self.on_settings_delete)

            outer = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, spacing=12)
            outer.set_border_width(16)

            label = Gtk.Label(label="Settings")
            label.set_xalign(0)
            outer.pack_start(label, False, False, 0)

            link = Gtk.LinkButton(uri="https://buildermark.dev", label="buildermark.dev")
            link.set_halign(Gtk.Align.START)
            outer.pack_start(link, False, False, 0)

            self.settings_window.add(outer)

        self.settings_window.show_all()
        self.settings_window.present()

    def on_settings_delete(self, _window: Gtk.Window, _event) -> bool:
        if self.settings_window is not None:
            self.settings_window.hide()
        return True

    def on_quit_clicked(self, *_args) -> None:
        self.stop_server()
        Gtk.main_quit()

    def stop_server(self) -> None:
        if self.server_proc is None:
            return

        if self.server_proc.poll() is None:
            self.server_proc.send_signal(signal.SIGTERM)
            for _ in range(20):
                if self.server_proc.poll() is not None:
                    break
                time.sleep(0.1)
            if self.server_proc.poll() is None:
                self.server_proc.kill()


def main() -> None:
    app = BuildermarkTrayApp()
    app.update_status()
    Gtk.main()
    app.stop_server()


if __name__ == "__main__":
    main()
