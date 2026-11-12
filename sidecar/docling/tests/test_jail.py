# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

import os
import tempfile
import unittest
from pathlib import Path

from goso_docling.jail import PathDenied, RawMissing, resolve_raw


class JailTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        raw = self.root / "t" / "minh-an" / "raw"
        raw.mkdir(parents=True)
        (raw / "abc.pdf").write_bytes(b"%PDF-1.4\n")
        (self.root / "t" / "minh-an" / "wiki").mkdir()
        (self.root / "t" / "minh-an" / "wiki" / "secret.md").write_text("no", encoding="utf-8")

    def tearDown(self):
        self.tmp.cleanup()

    def test_raw_file_resolves(self):
        path = resolve_raw(str(self.root), "minh-an", "raw/abc.pdf")
        self.assertTrue(str(path).endswith(os.path.join("raw", "abc.pdf")))

    def test_rejects_escape_and_other_trees(self):
        for tenant, raw in [
            ("minh-an", "../wiki/secret.md"),
            ("minh-an", "wiki/secret.md"),
            ("minh-an", "raw/../../wiki/secret.md"),
            ("../minh-an", "raw/abc.pdf"),
            ("minh-an", "raw/nested/abc.pdf"),
        ]:
            with self.assertRaises(PathDenied):
                resolve_raw(str(self.root), tenant, raw)

    def test_missing_is_not_a_read_of_another_path(self):
        with self.assertRaises(RawMissing):
            resolve_raw(str(self.root), "minh-an", "raw/missing.pdf")

    def test_symlink_is_denied(self):
        raw = self.root / "t" / "minh-an" / "raw"
        outside = self.root / "outside.pdf"
        outside.write_bytes(b"%PDF-1.4\n")
        link = raw / "link.pdf"
        link.symlink_to(outside)
        with self.assertRaises(PathDenied):
            resolve_raw(str(self.root), "minh-an", "raw/link.pdf")


if __name__ == "__main__":
    unittest.main()
