#!/bin/sh
# ============================================================
# 可选:生成 .env(Docker Manager License)
#
# 说明:现在**无需 .env 也能直接启动**——
#   JWT_SECRET / 管理员密码 首次启动自动生成并打印在日志里。
# 本脚本仅用于自定义 POSTGRES_PASSWORD(数据库密码,默认 licensepass)。
#
# 用法:
#   ./setup-env.sh            # 生成 .env(已存在则跳过)
#   ./setup-env.sh -f         # 强制重新生成
# 然后: docker compose up -d
# ============================================================
set -e

ENV_FILE="${ENV_FILE:-.env}"

if [ -f "$ENV_FILE" ] && grep -q '^POSTGRES_PASSWORD=.\+' "$ENV_FILE" 2>/dev/null && [ "$1" != "-f" ]; then
    echo ".env 已存在且 POSTGRES_PASSWORD 已设置,跳过(如需覆盖用 ./setup-env.sh -f)"
    exit 0
fi

umask 077
cat > "$ENV_FILE" <<EOF
POSTGRES_PASSWORD=$(openssl rand -hex 16)
EOF
chmod 600 "$ENV_FILE"

echo "✅ 已生成 $ENV_FILE(权限 600)"
echo "下一步: docker compose up -d"
echo "首次启动后查看日志获取管理员密码: docker compose logs license-server | grep -A2 密码"
