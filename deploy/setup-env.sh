#!/bin/sh
# ============================================================
# 一键生成 .env(随机强密钥),Docker Manager License 部署用
#
# 用法:
#   cd <compose 目录>
#   ./setup-env.sh            # 生成 .env(已存在且含 JWT_SECRET 则跳过)
#   ./setup-env.sh -f         # 强制重新生成
#
# 生成后:
#   docker compose up -d
#   docker compose logs -f license-server   # 日志中打印 PUBLIC KEY,保存用于集成
# ============================================================
set -e

ENV_FILE="${ENV_FILE:-.env}"

if [ -f "$ENV_FILE" ] && grep -q '^JWT_SECRET=.\+' "$ENV_FILE" 2>/dev/null && [ "$1" != "-f" ]; then
    echo ".env 已存在且 JWT_SECRET 已设置,跳过生成(如需覆盖用 ./setup-env.sh -f)"
    exit 0
fi

umask 077

gen() { openssl rand -hex "${1:-32}"; }

cat > "$ENV_FILE" <<EOF
POSTGRES_PASSWORD=$(gen 16)
JWT_SECRET=$(gen 32)
LICENSE_KEY_ID=2026-01
ADMIN_USERNAME=admin
ADMIN_PASSWORD=$(gen 8)Zz1!
EOF

chmod 600 "$ENV_FILE"
echo "✅ 已生成 $ENV_FILE(权限 600)"
echo ""
echo "管理员账号: $(grep '^ADMIN_USERNAME=' "$ENV_FILE" | cut -d= -f2)"
echo "管理员密码: $(grep '^ADMIN_PASSWORD=' "$ENV_FILE" | cut -d= -f2)"
echo ""
echo "下一步: docker compose up -d"
echo "查看公钥: docker compose logs -f license-server"
