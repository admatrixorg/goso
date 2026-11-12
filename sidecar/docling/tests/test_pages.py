# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

import time
import unittest

from goso_docling.pages import PageFailed, run_pages


class PageTimeoutTest(unittest.TestCase):
    def test_hung_page_times_out_without_sticking(self):
        calls = {"n": 0}

        def hung(page, timeout):
            calls["n"] += 1
            raise TimeoutError(page)

        started = time.monotonic()
        result = run_pages(1, hung, timeout=0.01, attempts=2)
        self.assertLess(time.monotonic() - started, 1)
        self.assertEqual(result["error_class"], "timeout")
        self.assertEqual(calls["n"], 2)
        self.assertEqual(result["ocr_engine"], "tesseract")
        self.assertEqual(result["ocr_lang"], "vie+eng")

    def test_second_attempt_succeeds(self):
        calls = {"n": 0}

        def flaky(page, timeout):
            calls["n"] += 1
            if calls["n"] == 1:
                raise TimeoutError(page)
            return "Hợp đồng thuê nhà tại Đà Nẵng"

        result = run_pages(1, flaky, timeout=1, attempts=2)
        self.assertEqual(result["error_class"], "")
        self.assertIn("Đà Nẵng", result["markdown"])
        self.assertEqual(result["page_count"], 1)

    def test_ocr_failed_is_not_retried_as_timeout(self):
        calls = {"n": 0}

        def failed(page, timeout):
            calls["n"] += 1
            raise PageFailed("ocr_failed")

        result = run_pages(2, failed, timeout=1, attempts=2)
        self.assertEqual(result["error_class"], "ocr_failed")
        self.assertEqual(calls["n"], 1)


if __name__ == "__main__":
    unittest.main()
