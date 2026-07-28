FROM docker.io/library/node:24.13.0-alpine3.23 AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm install --global npm@11.18.0 \
      --ignore-scripts --no-audit --no-fund \
      --fetch-retries=5 \
      --fetch-retry-mintimeout=20000 \
      --fetch-retry-maxtimeout=120000
RUN --mount=type=cache,target=/root/.npm \
    npm ci --ignore-scripts --no-audit --no-fund --maxsockets=4 \
      --fetch-retries=5 \
      --fetch-retry-mintimeout=20000 \
      --fetch-retry-maxtimeout=120000
COPY frontend/ ./
COPY internal/web/translation /src/internal/web/translation
RUN npm run build

FROM docker.io/library/golang:1.26.5-alpine3.23 AS panel-builder
RUN apk add --no-cache build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal/ ./internal/
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
ENV CGO_ENABLED=1 GOOS=linux GOARCH=amd64
RUN go build \
    -trimpath \
    -buildvcs=false \
    -tags "netgo osusergo" \
    -ldflags "-s -w -linkmode external -extldflags -static" \
    -o /out/x-ui .

FROM docker.io/library/alpine:3.23.3 AS runtime-assets
RUN apk add --no-cache ca-certificates curl tar tzdata unzip
WORKDIR /assets

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o xray.zip \
      https://github.com/XTLS/Xray-core/releases/download/v26.7.28/Xray-linux-64.zip \
    && echo "8195d909f1109b8f3d99eefe401a3c451d7bf4af71f24d3815420f77e5dd2a40  xray.zip" | sha256sum -c - \
    && unzip -q xray.zip xray \
    && install -Dm755 xray /out/bin/xray-linux-amd64

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o mtg.tar.gz \
      https://github.com/MHSanaei/mtg-multi/releases/download/v1.15.0/mtg-multi-1.15.0-linux-amd64.tar.gz \
    && echo "f1f8763504753fb863a0ddff83eab19c856747289c376275c44b717f1747908e  mtg.tar.gz" | sha256sum -c - \
    && tar -xzf mtg.tar.gz \
    && install -Dm755 mtg-multi-1.15.0-linux-amd64/mtg-multi /out/bin/mtg-linux-amd64

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geoip.dat \
      https://github.com/Loyalsoldier/v2ray-rules-dat/releases/download/202607252248/geoip.dat \
    && curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geosite.dat \
      https://github.com/Loyalsoldier/v2ray-rules-dat/releases/download/202607252248/geosite.dat \
    && echo "cdf411fce977a1f48adb6a3b224e3e2bd7eccfcd4d6e2e30c6dc443f1a0e8e52  /out/bin/geoip.dat" | sha256sum -c - \
    && echo "27c8353b72f5cbde081976ebbfda9bec0dba893448b6b729b1b2b6ba7f74af5e  /out/bin/geosite.dat" | sha256sum -c -

FROM docker.io/library/alpine:3.23.3
RUN apk add --no-cache ca-certificates nginx nginx-mod-stream tzdata
WORKDIR /app
COPY --from=panel-builder /out/x-ui /app/x-ui
COPY --from=runtime-assets /out/bin/ /app/bin/
ENV TZ=Asia/Singapore \
    XUI_BIN_FOLDER=/app/bin \
    XUI_DB_FOLDER=/etc/x-ui \
    XUI_LOG_FOLDER=/etc/x-ui/log \
    XUI_PORT=2053
VOLUME ["/etc/x-ui"]
ENTRYPOINT ["/app/x-ui"]
CMD ["run"]
