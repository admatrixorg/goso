# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

"""Convert one raw file. Missing models return not_configured and do not download."""

import os
import subprocess
import sys
from pathlib import Path

from goso_docling.pages import PageFailed, run_pages

_TESSDATA = ("vie.traineddata", "eng.traineddata")


def artifacts_ready(path=None) -> bool:
    root = path if path is not None else os.environ.get("DOCLING_ARTIFACTS_PATH", "")
    if not root:
        return False
    folder = Path(root)
    if not folder.is_dir():
        return False
    return any(child.is_file() for child in folder.rglob("*"))


def tessdata_ready() -> bool:
    roots = []
    prefix = os.environ.get("TESSDATA_PREFIX", "").strip()
    if prefix:
        roots.append(Path(prefix))
    roots.extend(
        [
            Path("/usr/share/tesseract-ocr/5/tessdata"),
            Path("/usr/share/tesseract-ocr/4.00/tessdata"),
            Path("/usr/share/tessdata"),
        ]
    )
    for root in roots:
        if all((root / name).is_file() for name in _TESSDATA):
            return True
    return False


def offline_env():
    env = os.environ.copy()
    env["HF_HUB_OFFLINE"] = "1"
    env["TRANSFORMERS_OFFLINE"] = "1"
    env["HF_DATASETS_OFFLINE"] = "1"
    env["PYTHONDONTWRITEBYTECODE"] = "1"
    return env


def kind_of(mime: str, path: Path) -> str:
    ext = path.suffix.lower()
    mime = (mime or "").lower()
    if ext == ".pdf" or mime == "application/pdf":
        return "pdf"
    if ext in {".png", ".jpg", ".jpeg", ".gif", ".webp"} or mime.startswith("image/"):
        return "image"
    if ext in {".docx", ".xlsx", ".pptx"}:
        return "office"
    return "unsupported"


def page_count(path: Path, kind: str) -> int:
    if kind in {"image", "office"}:
        return 1
    if kind != "pdf":
        return 0
    try:
        import pypdfium2 as pdfium
    except ImportError:
        return 1
    doc = pdfium.PdfDocument(str(path))
    try:
        return len(doc)
    finally:
        doc.close()


def convert_document(path: Path, mime: str, page_timeout: float, attempts: int) -> dict:
    if not artifacts_ready() or not tessdata_ready():
        return _not_configured()
    kind = kind_of(mime, path)
    if kind == "unsupported":
        return {**_not_configured(), "error_class": "unsupported"}
    pages = page_count(path, kind)
    if pages < 1:
        return {**_not_configured(), "error_class": "empty_conversion", "page_count": 0}

    def one(page, timeout):
        return _convert_page_process(path, kind, page, timeout)

    return run_pages(pages, one, page_timeout, attempts)


def _not_configured():
    return {
        "markdown": "",
        "page_count": 0,
        "ocr_engine": "tesseract",
        "ocr_lang": "vie+eng",
        "error_class": "not_configured",
    }


def _convert_page_process(path: Path, kind: str, page: int, timeout: float) -> str:
    try:
        proc = subprocess.run(
            [sys.executable, "-m", "goso_docling.onepage", str(path), kind, str(page)],
            timeout=timeout,
            capture_output=True,
            check=False,
            env=offline_env(),
        )
    except subprocess.TimeoutExpired as exc:
        raise TimeoutError(page) from exc
    if proc.returncode == 0:
        return proc.stdout.decode("utf-8")
    mapping = {2: "ocr_failed", 3: "unsupported", 4: "empty_conversion", 5: "too_large"}
    raise PageFailed(mapping.get(proc.returncode, "internal"))
