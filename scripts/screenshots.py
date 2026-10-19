#!/usr/bin/env python3
"""Drive real GCRT views and capture their complete terminal screens.

All four app directories and both tmux servers live under a private directory.
SSH is a local stand-in; no personal configuration or network hosts are used.
"""
import fcntl
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

import pyte
from render import render

COLS, ROWS = 120, 34
binary = str(Path(sys.argv[1]).resolve())
output = Path(sys.argv[2]).resolve()
work = Path(tempfile.mkdtemp(prefix="gcrt-screenshots-"))
env = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor", TMUX_TMPDIR=str(work))
for key in ("TMUX", "TMUX_PANE", "GCRT_WORKSPACE", "NO_COLOR"):
    env.pop(key, None)
for kind in ("config", "state", "data", "cache"):
    (work / kind).mkdir()
fake = work / "demo-ssh"
fake.write_text('''#!/bin/sh
printf '\033[38;5;114mDemo SSH session\033[0m\n\n'
printf 'Target: %s\n\n' "$*"
printf 'This local demo transport does not access the network.\n'
printf 'The session tree, tabs, menus and controls are live GCRT.\n\n'
printf 'admin@demo:~$ '
exec sleep 300
''')
fake.chmod(0o700)
(work / "config/config.toml").write_text(
    '[general]\ndisplay_mode="workspace"\nworkspace_layout="sidebar"\n'
    'workspace_tree_width=40\n[transports.ssh]\nbinary=' + json.dumps(str(fake)) + '\n'
)
hosts = [
    ("core-sw-01", "network/switches", "192.0.2.11"),
    ("dist-sw-02", "network/switches", "192.0.2.12"),
    ("edge-rtr-01", "network/routers", "192.0.2.21"),
    ("bastion", "network/routers", "192.0.2.22"),
    ("pve-01", "servers/virt", "192.0.2.31"),
    ("k3s-01", "servers/k8s", "192.0.2.41"),
    ("k3s-02", "servers/k8s", "192.0.2.42"),
    ("app-01", "servers/apps", "192.0.2.51"),
    ("build-01", "servers/apps", "192.0.2.52"),
    ("nas-01", "storage", "192.0.2.61"),
]
(work / "config/sessions.toml").write_text("\n".join(
    f'[[session]]\nid="00000000-0000-4000-8000-{i:012d}"\n'
    f'name="{name}"\nslug="{name}"\ntransport="ssh"\ngroup="{group}"\n'
    f'[session.ssh]\nhost="{host}"\nuser="admin"\n'
    for i, (name, group, host) in enumerate(hosts, 1)
))
class CaptureScreen(pyte.Screen):
    # tmux 3.7 sends private status queries which pyte 0.8 cannot dispatch to
    # its stock handler. Queries don't change pixels; pump handles replies.
    def report_device_status(self, mode, **kwargs):
        pass


screen = CaptureScreen(COLS, ROWS)
stream = pyte.ByteStream(screen)
transcript = bytearray()
child = fd = None
shots = []


def tm(*args, tabs=False, check=True):
    return subprocess.run(
        ["tmux", "-L", "gcrt-tabs" if tabs else "gcrt", *args], env=env,
        capture_output=True, text=True, timeout=5, check=check,
    ).stdout.rstrip("\n")


def pump(seconds=0.2):
    until = time.monotonic() + seconds
    while time.monotonic() < until:
        if select.select([fd], [], [], 0.02)[0]:
            try:
                data = os.read(fd, 65536)
            except OSError:
                return
            if not data:
                return
            transcript.extend(data)
            stream.feed(data)
            if b"\x1b]11;?" in data:
                os.write(fd, b"\x1b]11;rgb:2828/2c2c/3434\x07")
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")


def waitfor(predicate, detail):
    until = time.monotonic() + 8
    while time.monotonic() < until:
        pump()
        try:
            if predicate():
                return
        except subprocess.CalledProcessError:
            pass
    raise AssertionError(detail + "\n" + "\n".join(screen.display))


def send(data):
    os.write(fd, data)
    pump()


def capture(pane="%0"):
    return tm("capture-pane", "-p", "-t", pane)


def click(label, pane="%0", button=0):
    waitfor(lambda: label in capture(pane), "Missing click target: " + label)
    for y, line in enumerate(capture(pane).splitlines()):
        if label not in line:
            continue
        left, top = map(int, tm("display-message", "-p", "-t", pane,
                               "#{pane_left} #{pane_top}").split())
        x, y = left + line.index(label), top + y
        send(f"\x1b[<{button};{x+1};{y+1}M".encode())
        send(f"\x1b[<{button};{x+1};{y+1}m".encode())
        return


def shot(name, expected, forbidden=()):
    # Read the client's current grid, including its native popup overlays.
    waitfor(lambda: all(text in "\n".join(screen.display) for text in expected),
            "Wrong view for " + name)
    text = "\n".join(screen.display)
    assert not any(word in text for word in forbidden), "Unexpected content in " + name
    assert "Sessions · F2" in text, "Screenshot omitted the recovery control"
    grid = {"columns": COLS, "lines": ROWS, "cells": [
        [screen.buffer[y][x]._asdict() for x in range(COLS)] for y in range(ROWS)
    ]}
    (work / (name + ".json")).write_text(json.dumps(grid))
    (work / (name + ".txt")).write_text(text)
    render(grid, work / (name + ".png"))
    shots.append(name)
    print("Verified view:", name, flush=True)


try:
    child, fd = pty.fork()
    if child == 0:
        args = [binary]
        for kind in ("config", "state", "data", "cache"):
            args += ["-" + kind + "-dir", str(work / kind)]
        os.execve(binary, args, env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
    os.set_blocking(fd, False)
    waitfor(lambda: "Quick" in capture(), "Session tree did not start")
    waitfor(lambda: "no connections open yet" in capture("%1"), "Idle view missing")
    shot("idle", ["Quick", "no connections open yet", "click a session in the tree"])

    for host in ("core-sw-01", "k3s-01"):
        send(b"\x1bOQ")  # F2: focus tree
        click(host)
        waitfor(lambda: "Demo SSH session" in capture("%1"), "Demo did not connect")
    waitfor(lambda: len(tm("list-windows", tabs=True).splitlines()) == 2, "Expected two tabs")
    send(b"\x1bOQ")
    shot("sidebar", ["core-sw-01", "k3s-01", "Demo SSH session"], ["no connections open yet"])

    click("core-sw-01", button=2)
    shot("menu", ["choose an action", "Reconnect", "Edit", "Duplicate", "[ Cancel ]"])
    click("Edit")
    shot("form", ["edit core-sw-01", "192.0.2.11", "[ Save ]", "[ Cancel ]"])
    click("[ Cancel ]")

    click("Quick")
    send(b"admin@192.0.2.80:2222")
    shot("quick-connect", ["Quick Connect", "admin@192.0.2.80:2222", "[ Connect ]", "[ Cancel ]"])
    click("[ Cancel ]")

    # Same zoom a user gets with Ctrl-b z; help is scrollable at smaller sizes.
    tm("resize-pane", "-Z", "-t", "%0")
    click("Help")
    shot("help", ["ghosttycrt — help", "Quick Connect", "F2 / Sessions", "[ Close ]"])
    click("[ Close ]")
    tm("resize-pane", "-Z", "-t", "%0")
    pump(0.8)  # let both nested clients settle after unzoom before opening a menu

    click("core-sw-01", button=2)
    click("Reconnect")
    shot("reconnect", ["Reconnect core-sw-01?", "[ Yes ]", "[ No ]"])
    click("[ No ]")
    assert len(tm("list-windows", tabs=True).splitlines()) == 2, "Capture closed a demo connection"

    # Only publish the complete, verified set. Keep captures for visual review.
    output.mkdir(parents=True, exist_ok=True)
    for name in shots:
        shutil.copy2(work / (name + ".png"), output / (name + ".png"))
    print(f"Wrote {len(shots)} verified screenshots to {output}", flush=True)
finally:
    (work / "transcript.bin").write_bytes(transcript)
    tm("kill-server", tabs=True, check=False)
    tm("kill-server", check=False)
    if fd is not None:
        os.close(fd)
    if child:
        for _ in range(20):
            try:
                if os.waitpid(child, os.WNOHANG)[0]:
                    break
            except ChildProcessError:
                break
            time.sleep(0.05)
        else:
            os.kill(child, signal.SIGKILL)
            os.waitpid(child, 0)
    print("Capture artifacts:", work, flush=True)
