# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

import inspect
import os
import unittest
from pathlib import Path

from goso_docling import convert
from goso_docling.convert import convert_document


class OfflineTest(unittest.TestCase):
    def test_missing_artifacts_do_not_download(self):
        source = inspect.getsource(convert) + inspect.getsource(convert_document)
        self.assertNotIn("download_models", source)
        self.assertNotIn("wget", source)
        self.assertNotIn("curl", source)
        old_art = os.environ.pop("DOCLING_ARTIFACTS_PATH", None)
        old_tess = os.environ.pop("TESSDATA_PREFIX", None)
        try:
            result = convert_document(Path("hop-dong.pdf"), "application/pdf", 1, 2)
        finally:
            if old_art is not None:
                os.environ["DOCLING_ARTIFACTS_PATH"] = old_art
            if old_tess is not None:
                os.environ["TESSDATA_PREFIX"] = old_tess
        self.assertEqual(result["error_class"], "not_configured")
        self.assertEqual(result["markdown"], "")
        self.assertEqual(result["ocr_engine"], "tesseract")
        self.assertEqual(result["ocr_lang"], "vie+eng")


if __name__ == "__main__":
    unittest.main()
