FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /hass-dashboard ./cmd/hass-dashboard


FROM alpine:3.19 AS runtime

LABEL org.opencontainers.image.source="https://github.com/markis/hass-dashboard" \
    org.opencontainers.image.description="Home Assistant Dashboard - generates dashboard images from calendar and weather data" \
    org.opencontainers.image.licenses="MIT"

RUN --mount=type=cache,target=/var/cache/apk \
    apk add \
    chromium \
    font-noto \
    fontconfig \
    ca-certificates \
    tzdata \
    yq \
    && fc-cache -f -v \
    && mkdir -p /tmp/chrome-data /tmp/chrome-crashpad && chmod 777 /tmp/chrome-data /tmp/chrome-crashpad

# Set chromium path for chromedp
ENV CHROME_PATH=/usr/bin/chromium-browser
# Disable Chrome crash reporting completely
ENV CHROME_CRASHPAD_HANDLER_DISABLE=1
ENV CHROME_CRASHPAD_PIPE_NAME=
ENV BREAKPAD_DUMP_LOCATION=/dev/null
ENV CHROME_NO_FIRST_RUN=1

WORKDIR /app

COPY scripts/healthcheck.sh /app/scripts/healthcheck.sh
COPY --from=builder /hass-dashboard /app/hass-dashboard

USER 65534:65534
ENTRYPOINT ["/app/hass-dashboard", "--config", "/app/config.yaml"]
