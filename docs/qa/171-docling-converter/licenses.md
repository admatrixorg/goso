# SPEC 171 — licenses for the sidecar image

This table is the intended bill of materials. The image was not built in this PR, so the rows are not an inventory of files inside an image. Prod stays off until someone builds the image and checks this table against the files in it.

| Piece | What the Dockerfile asks for | License as published | How it was checked on 12 Nov 2026 |
|---|---|---|---|
| Docling Python package 2.150.0 | `requirements.txt` pins `docling==2.150.0` | MIT | PyPI `docling` 2.150.0 lists License MIT. The series overview already recorded the GitHub LICENSE as MIT |
| Tesseract engine | apt `tesseract-ocr` in the image | Apache-2.0 is the upstream project license | Not opened from the Debian copyright file of the package that would be installed. The image was not built |
| `eng.traineddata` and `vie.traineddata` | apt `tesseract-ocr-eng` and `tesseract-ocr-vie` | Apache-2.0 on `tesseract-ocr/tessdata` | The tessdata README says all data in that repository are under Apache-2.0, and the LICENSE file is Apache-2.0. Ubuntu packages ship `tessdata_fast`. That package's copyright file was not opened here, and the traineddata bytes were not hashed |
| Layout, TableFormer, picture classifier, code/formula models | `prefetch.py` calls `download_models` with those four flags on, during `docker build` only | Not verified file by file | `docling.utils.model_downloader.download_models` in current `main` downloads those models when the matching flags are true. The license files in the Hugging Face repos were not opened. A built image was not unpacked |
| EasyOCR package and weights | Must not be installed. `prefetch.py` passes `with_easyocr=False` and exits if `easyocr` is importable. The build also exits if `easyocr` is importable after `pip install` | n/a | The check is in the Dockerfile and `prefetch.py`. It was not executed, because the image was not built |
| RapidOCR weights | `with_rapidocr=False`. `prefetch.py` exits if a downloaded path contains `rapidocr` | n/a | Same as EasyOCR: the check is in source, not in a built image. The sidecar never selects RapidOCR. Whether `pip install docling==2.150.0` still installs a RapidOCR Python package was not observed |
| ocrmac | Not referenced by the page worker | n/a | `onepage.py` selects `TesseractCliOcrOptions(lang=["vie", "eng"])` only |

The runtime stage does not run `prefetch.py`. Its command is `python -u -m goso_docling`.
