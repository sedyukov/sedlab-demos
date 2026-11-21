# Build context = repo root of sedlab-demos with the private modules checked out at ./modules-src (see CI workflow).
# Multi-stage: module sources exist only in the builder stage (never pushed). The final image gets just the
# installed packages in /opt/venv — no module source trees, tests, contract fixtures or build files in any layer.

FROM python:3.12-slim AS builder
ENV PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
WORKDIR /src
COPY modules-src/modules/ai/python ai
COPY modules-src/modules/telegram-core/python telegram-core
COPY demos/voice/pyproject.toml demo/pyproject.toml
COPY demos/voice/app demo/app
RUN pip wheel --no-deps -w /wheels ./ai ./telegram-core ./demo \
 && python -m venv /opt/venv \
 && /opt/venv/bin/pip install /wheels/*.whl \
 && find /opt/venv -name '__pycache__' -prune -exec rm -rf {} + \
 && find /opt/venv/lib -path '*site-packages/tests*' -prune -exec rm -rf {} + \
 && rm -f /opt/venv/bin/pip* /opt/venv/bin/activate*

FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PATH=/opt/venv/bin:$PATH \
    VOICE_SAMPLES_DIR=/app/samples VOICE_STATIC_DIR=/app/static \
    AI_GIGACHAT_CA_FILE=/app/certs/russian_trusted_root_ca.pem
WORKDIR /app
COPY --from=builder /opt/venv /opt/venv
COPY demos/voice/static /app/static
COPY demos/voice/samples /app/samples
COPY demos/voice/certs /app/certs
USER nobody
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=2)"
CMD ["uvicorn", "voice_demo.main:app", "--host", "0.0.0.0", "--port", "8080", "--workers", "1", "--no-server-header", "--timeout-keep-alive", "15"]
