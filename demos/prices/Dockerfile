# Build context: demos/prices (CI checks out the private modules into demos/prices/.modules-src).
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/prices . && rm -rf .modules-src

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/prices /prices
ENV ADDR=:8080 GOMEMLIMIT=90MiB
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/prices"]
