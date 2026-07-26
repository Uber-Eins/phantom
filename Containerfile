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
      https://github.com/XTLS/Xray-core/releases/download/v26.7.11/Xray-linux-64.zip \
    && echo "aa11c3685c71da0ffc71e511db50404609e7e963bb914b048f59a6a00af8930e  xray.zip" | sha256sum -c - \
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

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geoip_IR.dat \
      https://github.com/Chocolate4U/Iran-v2ray-rules/releases/download/202607260714/geoip.dat \
    && curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geosite_IR.dat \
      https://github.com/Chocolate4U/Iran-v2ray-rules/releases/download/202607260714/geosite.dat \
    && echo "ab26a6def89a7001ea7c927d5a97ace429eeaaea476f27c0d2c4f16c31d34add  /out/bin/geoip_IR.dat" | sha256sum -c - \
    && echo "0d1b3152c8a5cbbfd954956775c0d3497862ba2128731191677e11c7605abe31  /out/bin/geosite_IR.dat" | sha256sum -c -

RUN curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geoip_RU.dat \
      https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/download/202607261102/geoip.dat \
    && curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 -o /out/bin/geosite_RU.dat \
      https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/download/202607261102/geosite.dat \
    && echo "9c3ff6a4331fed855ed6b56d7deec0ce2a0a6e626a74c6910ddb2881b7e42bf9  /out/bin/geoip_RU.dat" | sha256sum -c - \
    && echo "5dd054face5cde77bf42350be2202ebb5170bc66e418922cd5e63c78e6804ea2  /out/bin/geosite_RU.dat" | sha256sum -c -

FROM scratch
WORKDIR /app
COPY --from=panel-builder /out/x-ui /app/x-ui
COPY --from=runtime-assets /out/bin/ /app/bin/
COPY --from=runtime-assets /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=runtime-assets /usr/share/zoneinfo/Asia/Singapore /usr/share/zoneinfo/Asia/Singapore
COPY --from=runtime-assets /tmp /tmp
ENV TZ=Asia/Singapore \
    XUI_BIN_FOLDER=/app/bin \
    XUI_DB_FOLDER=/etc/x-ui \
    XUI_LOG_FOLDER=/etc/x-ui/log \
    XUI_PORT=2053
VOLUME ["/etc/x-ui"]
ENTRYPOINT ["/app/x-ui"]
CMD ["run"]
