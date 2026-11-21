# Build context = repo root of sedlab-demos with the private modules checked out at ./modules-src (see CI workflow).
FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1 \
    VOICE_SAMPLES_DIR=/app/samples VOICE_STATIC_DIR=/app/static \
    AI_GIGACHAT_CA_FILE=/app/certs/russian_trusted_root_ca.pem
WORKDIR /app
COPY modules-src/modules/ai/python /tmp/m/ai
COPY modules-src/modules/telegram-core/python /tmp/m/telegram-core
COPY demos/voice/pyproject.toml /tmp/demo/pyproject.toml
COPY demos/voice/app /tmp/demo/app
RUN pip install /tmp/m/ai /tmp/m/telegram-core /tmp/demo && rm -rf /tmp/m /tmp/demo
COPY demos/voice/static /app/static
COPY demos/voice/samples /app/samples
COPY demos/voice/certs /app/certs
USER nobody
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=2)"
CMD ["uvicorn", "voice_demo.main:app", "--host", "0.0.0.0", "--port", "8080", "--workers", "1", "--no-server-header", "--timeout-keep-alive", "15"]
