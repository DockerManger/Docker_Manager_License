# ================= Stage 1: 前端构建 =================
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install --no-audit --no-fund --registry=https://registry.npmmirror.com
COPY web/ ./
RUN npm run build

# ================= Stage 2: Go 后端编译 =================
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN apk add --no-cache git ca-certificates
WORKDIR /app
RUN go env -w GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/
# web/embed.go 是 go:embed 入口,必须随包编译(漏拷会报 package web 找不到)
COPY web/embed.go ./web/
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o license-server ./cmd/license-server

# ================= Stage 3: 运行镜像(non-root) =================
FROM alpine:3.20
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="Docker Manager License"
LABEL org.opencontainers.image.source="https://github.com/MinimaxFlora/Docker_Manager_License"
LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.revision=${COMMIT}
LABEL org.opencontainers.image.created=${BUILD_TIME}
RUN apk add --no-cache ca-certificates tini \
    && addgroup -S license && adduser -S -G license license

COPY --from=build /app/license-server /usr/local/bin/license-server

# 私钥挂载目录(绝不 COPY 私钥进镜像)
RUN mkdir -p /private /data \
    && chown -R license:license /private /data

USER license
ENV SERVER_ADDR=:3000
ENV LICENSE_PRIVATE_KEY_PATH=/private/license.key
VOLUME ["/private", "/data"]
EXPOSE 3000

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["license-server"]
