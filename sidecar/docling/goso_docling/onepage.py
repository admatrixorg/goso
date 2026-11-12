# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.

"""Convert one page with Tesseract CLI vie+eng. Exits non-zero on failure.

This process does not download models. HF_HUB_OFFLINE is set before Docling
is imported. The only OCR engine selected here is Tesseract CLI.
"""

import os
import sys


def main(argv) -> int:
    os.environ["HF_HUB_OFFLINE"] = "1"
    os.environ["TRANSFORMERS_OFFLINE"] = "1"
    os.environ["HF_DATASETS_OFFLINE"] = "1"
    if len(argv) != 3:
        return 1
    path, kind, page_s = argv
    try:
        page_no = int(page_s)
    except ValueError:
        return 3
    try:
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import (
            PdfPipelineOptions,
            TesseractCliOcrOptions,
        )
        from docling.document_converter import DocumentConverter, PdfFormatOption
    except Exception:
        return 1
    artifacts = os.environ.get("DOCLING_ARTIFACTS_PATH", "")
    try:
        ocr = TesseractCliOcrOptions(lang=["vie", "eng"])
        opts = PdfPipelineOptions()
        opts.do_ocr = True
        opts.ocr_options = ocr
        if artifacts:
            opts.artifacts_path = artifacts
        if hasattr(opts, "enable_remote_services"):
            opts.enable_remote_services = False
        converter = DocumentConverter(
            format_options={
                InputFormat.PDF: PdfFormatOption(pipeline_options=opts),
                InputFormat.IMAGE: PdfFormatOption(pipeline_options=opts),
            }
        )
        kwargs = {}
        if kind == "pdf" and page_no > 0:
            kwargs["page_range"] = (page_no, page_no)
        result = converter.convert(path, **kwargs)
        sys.stdout.write(result.document.export_to_markdown())
    except Exception:
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
