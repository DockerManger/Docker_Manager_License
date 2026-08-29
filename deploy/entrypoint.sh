#!/bin/sh
# 单容器全包入口:PostgreSQL → license-server → nginx 依次启动。
# 数据/私钥目录属主修复后,各进程按各自用户降权运行(容器内无 root 常驻进程)。
#
# 两种模式:
#   A. 单容器全包(默认):未设置 DATABASE_URL 或指向 127.0.0.1 → 内置 PG
#   B. 外部 PG(compose 分离模式):DATABASE_URL 指向外部主机 → 跳过内置 PG
set -e

PGDATA="${PGDATA:-/var/lib/postgresql}"
PG_USER=postgres
APP_USER=license

# PostgreSQL 二进制目录(Alpine 3.20 postgresql16 包实际路径)
PG_BIN="/usr/libexec/postgresql16"

# 优雅停机:先停 PG(license-server 状态全在 PG,被 SIGKILL 也无妨)
cleanup() {
    echo "==> [entrypoint] shutting down PostgreSQL..."
    su-exec "$PG_USER" "$PG_BIN/pg_ctl" -D "$PGDATA" -m fast stop >/dev/null 2>&1 || true
}
trap cleanup TERM INT EXIT

echo "==> [entrypoint] fixing volume ownership..."
chown -R "$APP_USER:$APP_USER" /private /data 2>/dev/null || true

# 判断是否使用内置 PG:未设置 DATABASE_URL,或指向 127.0.0.1/localhost
USE_INTERNAL_PG=0
if [ -z "${DATABASE_URL:-}" ]; then
    USE_INTERNAL_PG=1
else
    case "$DATABASE_URL" in
        *"@127.0.0.1:"*|*"@localhost:"*) USE_INTERNAL_PG=1 ;;
    esac
fi

# ---------- 1. PostgreSQL ----------
if [ "$USE_INTERNAL_PG" = "1" ]; then
    mkdir -p "$PGDATA"
    chown -R "$PG_USER:$PG_USER" "$PGDATA"
    # PG 默认把 unix socket 锁文件写到 /run/postgresql,Alpine 容器内该目录不存在
    mkdir -p /run/postgresql
    chown "$PG_USER:$PG_USER" /run/postgresql

    if [ ! -f "$PGDATA/PG_VERSION" ]; then
        echo "==> [postgres] initializing data directory..."
        # --locale=C:Alpine 无完整 locale 环境,避免 initdb 因 locale 缺失失败
        su-exec "$PG_USER" "$PG_BIN/initdb" -D "$PGDATA" -U postgres --auth=trust --locale=C
    fi

    echo "==> [postgres] starting on 127.0.0.1:5432..."
    su-exec "$PG_USER" "$PG_BIN/pg_ctl" -D "$PGDATA" \
        -o "-c listen_addresses=127.0.0.1 -c port=5432" -w start

    # 确保 license 数据库存在(幂等)
    su-exec "$PG_USER" "$PG_BIN/createdb" -h 127.0.0.1 -U postgres license 2>/dev/null || true
    export DATABASE_URL="postgres://postgres@127.0.0.1:5432/license?sslmode=disable"
else
    echo "==> [postgres] using external PostgreSQL (DATABASE_URL provided)"
fi

# ---------- 2. license-server(自动迁移 + 首次生成私钥/管理员) ----------
echo "==> [license-server] starting on 127.0.0.1:3000..."
su-exec "$APP_USER" /usr/local/bin/license-server &
LS_PID=$!

# 等待 license-server 就绪(健康检查通过再起 nginx,避免反代 502)
echo "==> [entrypoint] waiting for license-server..."
for i in $(seq 1 30); do
    if su-exec "$APP_USER" wget -qO- http://127.0.0.1:3000/healthz 2>/dev/null | grep -q '"ok"'; then
        echo "==> [entrypoint] license-server healthy"
        break
    fi
    if ! kill -0 "$LS_PID" 2>/dev/null; then
        echo "!! [entrypoint] license-server exited prematurely" >&2
        exit 1
    fi
    sleep 1
done

# ---------- 3. nginx(前台常驻,反代 /license-api/ → license-server) ----------
echo "==> [nginx] starting on :80..."
/usr/sbin/nginx -g 'daemon off;' &
NGINX_PID=$!
wait "$NGINX_PID"
