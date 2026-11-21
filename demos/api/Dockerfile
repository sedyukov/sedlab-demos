# One image, two processes: the same image runs as the Go service (/app/api-go) and as the Python service (uvicorn).
# Build context = demos/api with the private modules checked out at ./.modules-src (CI: actions/checkout path).
# Multi-stage: module sources exist only in builder stages; the final image gets the compiled Go binary and
# installed wheels in /opt/venv (SECURITY.md p.5).

FROM golang:1.23-alpine AS go
WORKDIR /src/go
COPY .modules-src/modules/accounts/go /src/.modules-src/modules/accounts/go
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api-go .

FROM alpine:3.20 AS web
RUN apk add --no-cache curl
COPY tools/fetch-swagger.sh /fetch-swagger.sh
COPY web /web
RUN rm -rf /web/vendor && sh /fetch-swagger.sh /web

FROM python:3.12-slim AS py
ENV PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
WORKDIR /src
COPY .modules-src/modules/accounts/python accounts
COPY python/pyproject.toml app/pyproject.toml
COPY python/api_py app/api_py
RUN pip wheel --no-deps -w /wheels ./accounts ./app \
 && python -m venv /opt/venv \
 && /opt/venv/bin/pip install /wheels/*.whl \
 && find /opt/venv -name '__pycache__' -prune -exec rm -rf {} + \
 && rm -f /opt/venv/bin/pip* /opt/venv/bin/activate*

FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PATH=/opt/venv/bin:$PATH \
    DB_PATH=/data/api.db CONTRACT_DIR=/app/contract MIGRATIONS_DIR=/app/migrations WEB_DIR=/app/web \
    ADDR=:8080 GOMEMLIMIT=48MiB
WORKDIR /app
COPY --from=py /opt/venv /opt/venv
COPY --from=go /out/api-go /app/api-go
COPY --from=web /web /app/web
COPY contract /app/contract
COPY migrations /app/migrations
USER 1000:1000
EXPOSE 8080 8000
# default: the Python service; the Go service runs with `command: ["/app/api-go"]`
CMD ["uvicorn", "--factory", "api_py.app:create_app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1", "--no-server-header", "--no-access-log", "--timeout-keep-alive", "15"]
