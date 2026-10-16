#!/usr/bin/env python3
"""NONPRODUCTION: kernel-backed multi-UID Unix socket proof using forked FD handoff.

No real License Manager, key, customer traffic or external business effects.
Privileged test runner only; all children are synthetic, unprivileged UIDs.
"""
import errno
import json
import os
import socket
import struct
import tempfile
import unittest

OWNER = 24101
WORKER = 24102
OUTSIDER = 24103


def drop(uid):
    os.setgroups([])
    os.setgid(uid)
    os.setuid(uid)


def child(fn, uid):
    """Fork a disposable child; return (PID, read pipe); errors are serialized."""
    r, w = os.pipe()
    pid = os.fork()
    if pid == 0:
        os.close(r)
        try:
            drop(uid)
            out = fn()
            payload = {"ok": True, "result": out}
        except BaseException as e:
            payload = {"ok": False, "error": type(e).__name__ + ":" + str(e)}
        os.write(w, json.dumps(payload).encode())
        os.close(w)
        os._exit(0 if payload["ok"] else 1)
    os.close(w)
    return pid, r


def result(pid, r):
    try:
        output = os.read(r, 8192)
    finally:
        os.close(r)
    _, status = os.waitpid(pid, 0)
    return os.WIFEXITED(status) and os.WEXITSTATUS(status) == 0, json.loads(output)


@unittest.skipUnless(os.geteuid() == 0, "Root-required synthetic multi-UID kernel proof")
class SupervisorOwnedBoundary(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="lic-a9-phase28-")
        self.addCleanup(self.temp.cleanup)
        os.chmod(self.temp.name, 0o755)
        self.root = os.path.join(self.temp.name, "supervised")
        os.mkdir(self.root, 0o750)
        os.chown(self.root, 0, WORKER)
        os.chmod(self.root, 0o750)
        self.addr = os.path.join(self.root, "effect.sock")
        self.listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.listener.bind(self.addr)
        os.chown(self.addr, 0, WORKER)
        os.chmod(self.addr, 0o660)
        self.listener.listen(1)
        self.listener.settimeout(3)
        self.addCleanup(self.listener.close)

    def test_real_distinct_uid_worker_connects_to_owner_inherited_fd(self):
        def owner():
            assert os.geteuid() == OWNER
            # Owner has the listening FD but cannot traverse or replace the socket path.
            assert not os.access(self.root, os.X_OK)
            conn, _ = self.listener.accept()
            with conn:
                peer = struct.unpack("3i", conn.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
                assert peer[1] == WORKER, peer
                assert conn.recv(128) == b"SYNTHETIC_NO_EFFECT"
                conn.sendall(b"PEER_UID_CONFIRMED")
            return {"owner_uid": os.geteuid(), "peer_uid": peer[1]}
        pid_owner, r_owner = child(owner, OWNER)
        def worker():
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
                client.connect(self.addr)
                client.sendall(b"SYNTHETIC_NO_EFFECT")
                return client.recv(128).decode()
        pid_worker, r_worker = child(worker, WORKER)
        wk_ok, wk = result(pid_worker, r_worker)
        owner_ok, own = result(pid_owner, r_owner)
        self.assertTrue(wk_ok, wk)
        self.assertEqual(wk["result"], "PEER_UID_CONFIRMED")
        self.assertTrue(owner_ok, own)
        self.assertEqual(own["result"], {"owner_uid": OWNER, "peer_uid": WORKER})

    def test_same_owner_uid_cannot_unlink_or_replace_supervisor_socket(self):
        def attacker():
            denied = []
            for action in ("unlink", "bind"):
                try:
                    if action == "unlink":
                        os.unlink(self.addr)
                    else:
                        with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
                            s.bind(os.path.join(self.root, "alternate.sock"))
                except OSError as exc:
                    if exc.errno in (errno.EACCES, errno.EPERM):
                        denied.append(action)
                    else:
                        raise
            return denied
        pid, r = child(attacker, OWNER)
        ok, got = result(pid, r)
        self.assertTrue(ok, got)
        self.assertEqual(got["result"], ["unlink", "bind"])
        self.assertTrue(os.path.exists(self.addr))

    def test_outsider_uid_without_socket_group_is_denied(self):
        def outsider():
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as client:
                try:
                    client.connect(self.addr)
                except OSError as exc:
                    if exc.errno in (errno.EACCES, errno.EPERM):
                        return "DENIED_BY_KERNEL"
                    raise
                raise AssertionError("outsider connected")
        pid, r = child(outsider, OUTSIDER)
        ok, got = result(pid, r)
        self.assertTrue(ok, got)
        self.assertEqual(got["result"], "DENIED_BY_KERNEL")

    def test_supervisor_controls_namespace_while_owner_does_not(self):
        st = os.stat(self.root)
        sk = os.stat(self.addr)
        self.assertEqual(st.st_uid, 0)
        self.assertEqual(st.st_gid, WORKER)
        self.assertEqual(st.st_mode & 0o777, 0o750)
        self.assertEqual(sk.st_uid, 0)
        self.assertEqual(sk.st_gid, WORKER)
        self.assertEqual(sk.st_mode & 0o777, 0o660)

if __name__ == "__main__":
    unittest.main(verbosity=2)
