# Build context = repo root (CI checks the private modules out into ./modules-src).
FROM node:22-alpine AS web
WORKDIR /web
COPY demos/booking/web/package.json demos/booking/web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY demos/booking/web/ ./
RUN npm run build            # → /server/static

FROM golang:1.26-alpine AS go
WORKDIR /src/demos/booking/server
COPY modules-src/modules/telegram-core/go /src/modules-src/modules/telegram-core/go
COPY modules-src/modules/payments/go /src/modules-src/modules/payments/go
COPY modules-src/modules/notify/go /src/modules-src/modules/notify/go
COPY demos/booking/server/go.mod demos/booking/server/go.sum ./
RUN go mod download
COPY demos/booking/server/ ./
COPY --from=web /server/static ./static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /booking .

FROM gcr.io/distroless/static-debian12
COPY --from=go /booking /booking
ENV PORT=8080 DATA_DIR=/data GOMEMLIMIT=48MiB
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/booking", "healthcheck"]
ENTRYPOINT ["/booking"]
