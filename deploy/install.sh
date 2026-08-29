#!/bin/bash
# ============================================================
# Docker Manager License — 一键部署脚本
#
# 一条命令部署(仓库已公开,直接走 GitHub raw):
#   curl -fsSL https://raw.githubusercontent.com/MinimaxFlora/Docker_Manager_License/master/deploy/install.sh | bash
#
# 特性:
#   - 自动检测 Docker / Compose 环境
#   - 重复执行自动检测已部署状态并显示服务状态(不重复初始化)
#   - 首次部署自动生成 POSTGRES_PASSWORD(.env 0600)
#   - 自动配置 nginx 反代 server 块(整个域名 /license-api/ → 127.0.0.1:3000)
#   - 部署完成后打印:访问地址 / 管理员账号 / 初始密码 / 公钥
#
# 参数(通过环境变量):
#   DML_DIR=~/dml         部署目录(默认 ~/docker-manager-license)
#   DML_PORT=8080         对外端口(默认 3000)
#   DML_DOMAIN=lic.example.com  反代域名(设置后自动写 nginx server 块;
#                               留空则不配置 nginx,仅直连端口)
#   DML_NGINX=0           禁用 nginx 自动配置(默认 1=开启)
#   DML_FORCE=1           强制重新部署
#   DML_MIRROR=<prefix>   compose 下载镜像前缀(如 ghproxy,国内加速)
# ============================================================
set -e

# ---------- 配置 ----------
INSTALL_DIR="${DML_DIR:-$HOME/docker-manager-license}"
PORT="${DML_PORT:-3000}"
FORCE="${DML_FORCE:-0}"
DOMAIN="${DML_DOMAIN:-}"
DO_NGINX="${DML_NGINX:-1}"
# 仓库已公开,compose 直接从 GitHub raw 拉取;DML_MIRROR 提供国内加速前缀
COMPOSE_URLS=(
  "${DML_MIRROR:+$DML_MIRROR/}https://raw.githubusercontent.com/MinimaxFlora/Docker_Manager_License/master/deploy/docker-compose.yml"
  "https://raw.githubusercontent.com/MinimaxFlora/Docker_Manager_License/master/deploy/docker-compose.yml"
)

c() { printf '\033[36m%s\033[0m\n' "$*"; }
ok() { printf '\033[32m✓ %s\033[0m\n' "$*"; }
err() { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }
warn() { printf '\033[33m! %s\033[0m\n' "$*" >&2; }

# ---------- 1. 环境检测 ----------
c "==> 检查 Docker 环境..."
if ! command -v docker >/dev/null 2>&1; then
  err "未检测到 Docker。请先安装: https://docs.docker.com/engine/install/"
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  err "未检测到 Docker Compose 插件(v2)。请安装: https://docs.docker.com/compose/install/"
  exit 1
fi
ok "Docker $(docker --version | awk '{print $3}' | tr -d ',') / Compose 已就绪"

# ---------- 2. 部署目录 ----------
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"
c "==> 部署目录: $INSTALL_DIR"

# ---------- nginx 反代配置(幂等,供首次部署与重复执行共用) ----------
configure_nginx() {
  [ "$DO_NGINX" = "1" ] || { c "nginx 自动配置已禁用(DML_NGINX=0),跳过"; return 0; }
  [ -n "$DOMAIN" ] || { c "未设置 DML_DOMAIN,跳过 nginx 反代配置(仅直连端口)"; return 0; }

  NGINX_BIN=""
  for p in /usr/sbin/nginx /usr/local/sbin/nginx /usr/bin/nginx; do
    [ -x "$p" ] && NGINX_BIN="$p" && break
  done
  if [ -z "$NGINX_BIN" ]; then
    warn "未检测到 nginx,跳过反代配置(服务仍可用 http://<IP>:$PORT)。安装后重跑本脚本即可自动补上。"
    return 0
  fi
  ok "nginx 已就绪($NGINX_BIN)"

  # 反代场景下,宿主端口只绑本机(公网流量经 nginx 80/443 进入)
  if grep -q '"3000:3000"' docker-compose.yml; then
    sed -i "s/\"3000:3000\"/\"127.0.0.1:$PORT:3000\"/" docker-compose.yml
    c "端口已收紧为仅本机监听(127.0.0.1:$PORT → 容器 3000)"
  fi

  NGINX_CONF="/etc/nginx/conf.d/dml-license.conf"
  CONF_CONTENT="$(cat <<EOF
# Docker Manager License 反代(由 install.sh 自动生成,重复执行自动更新)
server {
    listen 80;
    server_name $DOMAIN;

    location /license-api/ {
        proxy_pass http://127.0.0.1:$PORT/;   # 去掉 /license-api/ 前缀
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
)"

  if [ -f "$NGINX_CONF" ] && [ "$(cat "$NGINX_CONF")" = "$CONF_CONTENT" ]; then
    ok "nginx 反代配置已存在且一致,无需更新"
  else
    if ! echo "$CONF_CONTENT" > "$NGINX_CONF" 2>/dev/null; then
      warn "写入 $NGINX_CONF 失败(需要 root)。请手动执行: sudo bash $0 或手动放置该配置。"
      return 0
    fi
    ok "已写入 nginx 反代配置: $NGINX_CONF"
    if ! "$NGINX_BIN" -t 2>/dev/null; then
      warn "nginx 配置语法检查失败,请检查 $NGINX_CONF 与既有配置是否冲突"
      return 0
    fi
    if command -v systemctl >/dev/null 2>&1; then
      systemctl reload nginx 2>/dev/null || systemctl restart nginx 2>/dev/null || true
    else
      "$NGINX_BIN" -s reload 2>/dev/null || true
    fi
    ok "nginx 已重新加载"
  fi
}

# ---------- 3. 已部署检测(幂等) ----------
if [ "$FORCE" != "1" ] && [ -f docker-compose.yml ]; then
  if docker compose ps --status running 2>/dev/null | grep -q "license-server"; then
    c "==> 检测到已部署且运行中:"
    docker compose ps
    c "   访问: http://<服务器IP>:$PORT"
    c "   日志(含公钥): docker compose -f $INSTALL_DIR/docker-compose.yml logs license-server | grep -E 'PUBLIC KEY|密码'"
    configure_nginx
    c "   如需重新部署: DML_FORCE=1 $0"
    exit 0
  fi
fi

# ---------- 4. 下载 compose ----------
c "==> 下载 docker-compose.yml ..."
COMPOSE_OK=0
for url in "${COMPOSE_URLS[@]}"; do
  if curl -fsSL --connect-timeout 15 "$url" -o docker-compose.yml.tmp 2>/dev/null; then
    COMPOSE_OK=1
    break
  fi
done
if [ "$COMPOSE_OK" != "1" ]; then
  err "下载 compose 失败(网络问题可设置 DML_MIRROR 加速)。请重试或手动复制 deploy/docker-compose.yml"
  exit 1
fi
mv docker-compose.yml.tmp docker-compose.yml
ok "compose 下载完成"

# 端口自定义(容器内仍监听 3000,只改宿主映射)
if [ "$PORT" != "3000" ]; then
  sed -i "s/\"3000:3000\"/\"$PORT:3000\"/; s/SERVER_ADDR=:3000/SERVER_ADDR=:$PORT/" docker-compose.yml
  c "端口已改为 $PORT"
fi

# ---------- 5. .env(仅数据库密码;其余配置自动生成) ----------
if [ ! -f .env ]; then
  umask 077
  {
    echo "# Docker Manager License 环境变量(自动生成)"
    echo "POSTGRES_PASSWORD=$(openssl rand -hex 16)"
  } > .env
  chmod 600 .env
  ok ".env 已生成(数据库密码随机)"
else
  ok ".env 已存在,保留"
fi

# ---------- 6. 启动 ----------
c "==> 拉取镜像并启动(首次约 1-2 分钟)..."
COMPOSE_ARGS=""
# 配置了 Cloudflare Tunnel token 则启用 tunnel profile(HTTPS 访问)
if [ -n "${CF_TUNNEL_TOKEN:-}" ]; then
  {
    echo "CF_TUNNEL_TOKEN=$CF_TUNNEL_TOKEN"
  } >> .env
  COMPOSE_ARGS="--profile tunnel"
  ok "Cloudflare Tunnel 已启用(HTTPS)"
fi
docker compose $COMPOSE_ARGS up -d

# ---------- 7. nginx 反代(可选,默认开启) ----------
configure_nginx

# ---------- 8. 输出部署信息 ----------
sleep 6
echo ""
echo "=================================================================="
c "  Docker Manager License 部署完成"
if [ -n "$DOMAIN" ]; then
  c "  管理后台: http://<服务器IP>:$PORT  (反代已配置: https://$DOMAIN/license-api/)"
else
  c "  管理后台: http://<服务器IP>:$PORT"
fi
echo "------------------------------------------------------------------"
if docker compose logs license-server 2>/dev/null | grep -q "初始管理员已创建"; then
  c "  管理员账号(首次启动自动生成,请立即修改密码):"
  docker compose logs license-server 2>/dev/null | grep -E "用户名:|密码:" | tail -2
else
  c "  管理员账号: 使用之前部署时设置的密码(数据库未重置,未重新初始化)"
fi
echo "------------------------------------------------------------------"
c "  LICENSE PUBLIC KEY(集成 Docker_Manager_Go 用):"
docker compose logs license-server 2>/dev/null | grep -A1 "PUBLIC KEY" | tail -2
echo "------------------------------------------------------------------"
c "  查看完整日志: docker compose -f $INSTALL_DIR/docker-compose.yml logs -f license-server"
echo "=================================================================="
