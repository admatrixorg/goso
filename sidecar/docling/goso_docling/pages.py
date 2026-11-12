# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

"""Per-page timeout and retry. This module does not import Docling."""


class PageFailed(Exception):
    def __init__(self, error_class: str):
        super().__init__(error_class)
        self.error_class = error_class


def run_pages(page_count, convert_page, timeout, attempts):
    if page_count < 1:
        return _result(error_class="empty_conversion", page_count=0)
    if attempts < 1:
        attempts = 2
    if timeout <= 0:
        timeout = 60
    chunks = []
    for page in range(1, page_count + 1):
        finished = False
        for _ in range(attempts):
            try:
                text = convert_page(page, timeout)
            except TimeoutError:
                continue
            except PageFailed as exc:
                return _result(error_class=exc.error_class, page_count=page_count)
            chunks.append("" if text is None else text)
            finished = True
            break
        if not finished:
            return _result(error_class="timeout", page_count=page_count)
    return _result(markdown="\n\n".join(chunks), page_count=page_count)


def _result(markdown="", page_count=0, error_class=""):
    return {
        "markdown": markdown,
        "page_count": page_count,
        "ocr_engine": "tesseract",
        "ocr_lang": "vie+eng",
        "error_class": error_class,
    }
