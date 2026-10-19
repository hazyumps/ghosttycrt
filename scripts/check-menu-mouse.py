#!/usr/bin/env python3
"""Exercise real terminal mouse motion in tab menus using private test sessions.

Usage: python3 scripts/check-menu-mouse.py /absolute/path/to/gcrt
No real SSH command, configuration directory, or tmux socket is used.
"""
import fcntl
import os
import pathlib
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

binary = str(pathlib.Path(sys.argv[1]).resolve())
work = pathlib.Path(tempfile.mkdtemp(prefix="gcrt-menu-mouse-"))
env = dict(os.environ, TERM="xterm-256color", TMUX_TMPDIR=str(work))
for key in ("TMUX", "TMUX_PANE", "GCRT_WORKSPACE"):
    env.pop(key, None)
config = work / "config"
config.mkdir()
fake = work / "fake-ssh"
fake.write_text('#!/bin/sh\nprintf "CONNECTED %s\\n" "$*"\nexec sleep 300\n')
fake.chmod(0o700)
(config / "config.toml").write_text(
    '[general]\ndisplay_mode="workspace"\nworkspace_layout="sidebar"\n'
    'workspace_tree_width=34\n[transports.ssh]\nbinary="' + str(fake) + '"\n'
)
(config / "sessions.toml").write_text("")
transcript = bytearray()
child = fd = None


def tm(*args, tabs=False, check=True):
    return subprocess.run(
        ["tmux", "-L", "gcrt-tabs" if tabs else "gcrt", *args],
        env=env, capture_output=True, text=True, check=check, timeout=5,
    ).stdout.rstrip("\n")


def pump(seconds=0.15):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if select.select([fd], [], [], 0.02)[0]:
            try:
                data = os.read(fd, 65536)
            except OSError:
                return
            if not data:
                return
            transcript.extend(data)
            if b"\x1b]11;?" in data:
                os.write(fd, b"\x1b]11;rgb:2222/2222/2222\x07")
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[1;1R")


def waitfor(predicate, detail):
    deadline = time.monotonic() + 8
    while time.monotonic() < deadline:
        pump()
        try:
            if predicate():
                return
        except subprocess.CalledProcessError:
            pass
    raise AssertionError(detail)


def send(data):
    os.write(fd, data)
    pump()


def capture(pane="%1"):
    return tm("capture-pane", "-p", "-t", pane)


def coord(label, pane="%1"):
    for y, line in enumerate(capture(pane).splitlines()):
        if label in line:
            left, top = map(int, tm("display-message", "-p", "-t", pane,
                                   "#{pane_left} #{pane_top}").split())
            return left + line.index(label), top + y
    raise AssertionError("Missing " + label + "\n" + capture(pane))


def mouse(x, y, button, end="M"):
    send(f"\x1b[<{button};{x+1};{y+1}{end}".encode())


def click(label, pane="%1", button=0):
    x, y = coord(label, pane)
    mouse(x, y, button)
    mouse(x, y, button, "m")


def move_and_check(label):
    # Include unpressed motion over the connection and the menu choice.
    # Click-only tests miss tmux treating motion as the opening gesture's end.
    target = coord(label)
    for point in [(115, 25), (75, 12), target]:
        mouse(*point, 35)
        assert label in capture(), "Mouse motion dismissed " + label


def connections():
    return dict(line.split("\t") for line in tm(
        "list-panes", "-a", "-F", "#{@gcrt_slug}\t#{pane_pid}", tabs=True,
    ).splitlines())


try:
    child, fd = pty.fork()
    if child == 0:
        args = [binary]
        for kind in ("config", "state", "data", "cache"):
            args += ["-" + kind + "-dir", str(work / kind)]
        os.execve(binary, args, env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 32, 120, 0, 0))
    os.set_blocking(fd, False)
    waitfor(lambda: "Quick" in capture("%0"), "Tree did not render")
    for host in ("192.0.2.20", "192.0.2.21"):
        send(b"\x1bOQ")  # F2: tree
        click("Quick", "%0")
        send(host.encode())
        click("[ Connect ]", "%0")
    waitfor(lambda: len(connections()) == 2, "Missing fake connections")
    before = connections()
    for action in ("Close", "Reconnect"):
        click("192-0-2-20", button=2)
        move_and_check(action)
        click(action)
        move_and_check("No (cancel)")
        click("No (cancel)")
        assert connections() == before, action + " cancellation changed a connection"
        print("PASS:", action, "menu and confirmation survive mouse motion; No cancels", flush=True)
    click("192-0-2-20", button=2)
    move_and_check("Close")
    click("Close")
    move_and_check("Yes, close")
    click("Yes, close")
    waitfor(lambda: len(connections()) == 1, "Close did not remove the chosen fake session")
    assert connections()["192-0-2-21"] == before["192-0-2-21"], "Close affected the other session"
    print("PASS: mouse Close removes only the selected fake connection", flush=True)
finally:
    (work / "transcript.bin").write_bytes(transcript)
    for pane, name in [("%0", "tree"), ("%1", "content")]:
        try:
            (work / (name + ".txt")).write_text(capture(pane))
        except subprocess.CalledProcessError:
            pass
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
    print("Artifacts:", work, flush=True)
