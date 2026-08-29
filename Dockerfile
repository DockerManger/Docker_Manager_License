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

# ================= Stage 3: 运行镜像(单容器全包:license-server + nginx + PostgreSQL) =================
FROM alpine:3.20
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.title="Docker Manager License"
LABEL org.opencontainers.image.source="https://github.com/DockerManger/Docker_Manager_License"
LABEL org.opencontainers.image.version=${VERSION}
LABEL org.opencontainers.image.revision=${COMMIT}
LABEL org.opencontainers.image.created=${BUILD_TIME}
# nginx:反代 /license-api/ → 127.0.0.1:3000;postgresql16:内置数据库(仅监听 127.0.0.1)
# postgresql16-client:提供 createdb/psql(Alpine 中服务器包不含客户端工具)
# 单容器模式:用户只需 docker run -p 80:80 + 域名解析,无需宿主机 nginx
RUN apk add --no-cache ca-certificates tini su-exec nginx postgresql16 postgresql16-client \
    && addgroup -S license && adduser -S -G license license \
    && mkdir -p /run/nginx /private /data

COPY --from=build /app/license-server /usr/local/bin/license-server
# entrypoint:root 启动 PG → license-server → nginx,修复挂载目录属主后降权运行
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
# 内置 nginx 反代配置(/license-api/ 子路径方案,与 README 一致)
COPY deploy/nginx.conf /etc/nginx/nginx.conf
RUN chmod +x /usr/local/bin/entrypoint.sh \
    && chown -R license:license /private /data

ENV SERVER_ADDR=:3000
ENV DATA_DIR=/data
ENV LICENSE_PRIVATE_KEY_PATH=/private/license.key
# 内置 PG:数据目录(挂卷持久化)
ENV PGDATA=/var/lib/postgresql
VOLUME ["/private", "/data", "/var/lib/postgresql"]
EXPOSE 80

# entrypoint 需要 root 权限修复目录属主/启停 PG,再降权执行各进程
USER root
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
CMD ["license-server"]
