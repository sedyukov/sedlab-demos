# Build context = repo root (CI checks the private modules out into ./modules-src).
FROM golang:1.26-alpine AS go
WORKDIR /src/demos/auth
COPY modules-src/modules/accounts/go /src/modules-src/modules/accounts/go
COPY modules-src/modules/max-core/go /src/modules-src/modules/max-core/go
COPY modules-src/modules/notify/go /src/modules-src/modules/notify/go
COPY demos/auth/go.mod demos/auth/go.sum ./
RUN go mod download
COPY demos/auth/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /auth .

FROM gcr.io/distroless/static-debian12
COPY --from=go /auth /auth
ENV PORT=8080 DATA_DIR=/data GOMEMLIMIT=48MiB
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/auth", "healthcheck"]
ENTRYPOINT ["/auth"]
