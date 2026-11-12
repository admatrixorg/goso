# Copyright (c) 2026 MQ Global — GOSO. Clean-room sidecar.
# Build-time only. The runtime server does not import this module and does not
# download models. EasyOCR and RapidOCR weights are refused.

import importlib.util
import sys
from pathlib import Path

if importlib.util.find_spec("easyocr") is not None:
    sys.exit("easyocr is installed; this image must not include it")

from docling.utils.model_downloader import download_models

out = download_models(
    output_dir=Path("/opt/docling/models"),
    progress=False,
    with_layout=True,
    with_tableformer=True,
    with_code_formula=True,
    with_picture_classifier=True,
    with_rapidocr=False,
    with_easyocr=False,
    with_smolvlm=False,
    with_granitedocling=False,
    with_granitedocling_mlx=False,
    with_granitedocling_2stage=False,
    with_smoldocling=False,
    with_smoldocling_mlx=False,
    with_granite_vision=False,
    with_granite_chart_extraction=False,
)

bad = []
for path in Path(out).rglob("*"):
    low = str(path).lower()
    if "easyocr" in low or "rapidocr" in low:
        bad.append(str(path))
if bad:
    sys.exit("refusing easyocr or rapidocr artifacts")
