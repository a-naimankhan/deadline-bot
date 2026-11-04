# syntax=docker/dockerfile:1
# Сборка: статический бинарник без CGO. Версия Go = go.mod (go 1.27.1).
# --platform=$BUILDPLATFORM + GOARCH=$TARGETARCH: кросс-компиляция (например, под arm64-VM) без эмуляции.
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
WORKDIR /src

# Сначала только модули — слой кэшируется, пока не меняются go.mod/go.sum.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# Рантайм: distroless без shell и пакетного менеджера, с CA-сертификатами (HTTPS к Telegram).
# Миграции (migrations.FS) и tzdata вшиты в бинарник — копировать нечего.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/bot /bot
USER nonroot:nonroot
EXPOSE 8080
# curl в образе нет — проверяет сам бинарник: GET http://127.0.0.1:$PORT/healthz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
    CMD ["/bot", "-healthcheck"]
ENTRYPOINT ["/bot"]
