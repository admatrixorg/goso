# SPEC 171 Docling sidecar.
# Tessdata and Docling models are installed in this build. The runtime command
# is only the Python server. It does not wget, curl, or prefetch models.

FROM python:3.12-slim-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-vie \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/docling
COPY requirements.txt prefetch.py /opt/docling/
RUN pip install --no-cache-dir -r /opt/docling/requirements.txt \
    && python -c "import importlib.util,sys; sys.exit('easyocr must not be installed' if importlib.util.find_spec('easyocr') else 0)" \
    && python /opt/docling/prefetch.py

FROM python:3.12-slim-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        tesseract-ocr \
        tesseract-ocr-eng \
        tesseract-ocr-vie \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 docling \
    && useradd --uid 10001 --gid 10001 --create-home --shell /usr/sbin/nologin docling

COPY --from=build /usr/local /usr/local
COPY --from=build /opt/docling/models /opt/docling/models
COPY goso_docling /opt/docling/goso_docling

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/opt/docling \
    HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1 \
    HF_DATASETS_OFFLINE=1 \
    DOCLING_ARTIFACTS_PATH=/opt/docling/models \
    TESSDATA_PREFIX=/usr/share/tesseract-ocr/5/tessdata \
    HOME=/tmp \
    XDG_CACHE_HOME=/tmp \
    HF_HOME=/tmp \
    GOSO_DOCLING_BIND=0.0.0.0 \
    GOSO_DOCLING_PORT=8071

USER 10001:10001
EXPOSE 8071
ENTRYPOINT ["python", "-u", "-m", "goso_docling"]
