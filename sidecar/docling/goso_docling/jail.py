# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

"""Resolve one file under t/<tenant>/raw. Symlinks and escapes are refused."""

import os
from pathlib import Path


class PathDenied(Exception):
    pass


class RawMissing(Exception):
    pass


def resolve_raw(vault_root: str, tenant: str, raw_path: str) -> Path:
    if not vault_root or not tenant or not raw_path:
        raise PathDenied("path denied")
    if "/" in tenant or "\\" in tenant or tenant in (".", "..") or ".." in tenant:
        raise PathDenied("path denied")
    if not tenant.replace("-", "").replace("_", "").isalnum():
        raise PathDenied("path denied")
    raw_path = raw_path.replace("\\", "/").strip("/")
    if not raw_path.startswith("raw/"):
        raise PathDenied("path denied")
    name = raw_path[len("raw/") :]
    if not name or "/" in name or name in (".", "..") or ".." in name:
        raise PathDenied("path denied")
    root = Path(vault_root).resolve()
    cur = root
    for part in ("t", tenant, "raw", name):
        cur = cur / part
        if cur.is_symlink():
            raise PathDenied("path denied")
    raw_dir = root / "t" / tenant / "raw"
    try:
        cur.resolve().relative_to(raw_dir.resolve())
    except ValueError as exc:
        raise PathDenied("path denied") from exc
    if not cur.is_file():
        raise RawMissing("missing raw")
    if os.path.islink(cur):
        raise PathDenied("path denied")
    return cur
