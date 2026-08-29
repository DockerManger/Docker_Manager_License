#!/bin/bash
# ============================================================
# Docker Manager License — 一键部署脚本
#
# 一条命令部署:
#   curl -fsSL https://doc.kejizero.xyz/dml/install.sh | bash
#
# 特性:
#   - 自动检测 Docker / Compose 环境
#   - 重复执行自动检测已部署状态并显示服务状态(不重复初始化)
#   - 首次部署自动生成 POSTGRES_PASSWORD(.env 0600)
#   - 部署完成后打印:访问地址 / 管理员账号 / 初始密码 / 公钥
#
# 参数(通过环境变量):
#   DML_DIR=~/dml         部署目录(默认 ~/docker-manager-license)
#   DML_PORT=8080         对外端口(默认 3000)
#   DML_FORCE=1           强制重新部署
#   DML_MIRROR=<prefix>   compose 下载镜像前缀(如 ghproxy)
# ============================================================
set -e

# ---------- 配置 ----------
INSTALL_DIR="${DML_DIR:-$HOME/docker-manager-license}"
PORT="${DML_PORT:-3000}"
FORCE="${DML_FORCE:-0}"
# 脚本与 compose 托管在公开站点(仓库为 private,raw 不可公开访问)
SITE_BASE="${DML_SITE:-https://doc.kejizero.xyz/dml}"
COMPOSE_URLS=(
  "$SITE_BASE/docker-compose.yml"
  "${DML_MIRROR:+$DML_MIRROR/}https://raw.githubusercontent.com/MinimaxFlora/Docker_Manager_License/master/deploy/docker-compose.yml"
)

c() { printf '\033[36m%s\033[0m\n' "$*"; }
ok() { printf '\033[32m✓ %s\033[0m\n' "$*"; }
err() { printf '\033[31m✗ %s\033[0m\n' "$*" >&2; }

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

# ---------- 3. 已部署检测(幂等) ----------
if [ "$FORCE" != "1" ] && [ -f docker-compose.yml ]; then
  if docker compose ps --status running 2>/dev/null | grep -q "license-server"; then
    c "==> 检测到已部署且运行中:"
    docker compose ps
    c "   访问: http://<服务器IP>:$PORT"
    c "   日志(含公钥): docker compose -f $INSTALL_DIR/docker-compose.yml logs license-server | grep -E 'PUBLIC KEY|密码'"
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

# 端口自定义
if [ "$PORT" != "3000" ]; then
  sed -i "s/\"3000:3000\"/\"$PORT:3000\"/; s/SERVER_ADDR=:3000/SERVER_ADDR=:$PORT/" docker-compose.yml
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
docker compose up -d

# ---------- 7. 输出部署信息 ----------
sleep 6
echo ""
echo "================================================================"
c "  Docker Manager License 部署完成"
c "  管理后台: http://<服务器IP>:$PORT"
echo "----------------------------------------------------------------"
if docker compose logs license-server 2>/dev/null | grep -q "初始管理员已创建"; then
  c "  管理员账号(首次启动自动生成,请立即修改密码):"
  docker compose logs license-server 2>/dev/null | grep -E "用户名:|密码:" | tail -2
else
  c "  管理员账号: 使用之前部署时设置的密码(数据库未重置,未重新初始化)"
fi
echo "----------------------------------------------------------------"
c "  LICENSE PUBLIC KEY(集成 Docker_Manager_Go 用):"
docker compose logs license-server 2>/dev/null | grep -A1 "PUBLIC KEY" | tail -2
echo "----------------------------------------------------------------"
c "  查看完整日志: docker compose -f $INSTALL_DIR/docker-compose.yml logs -f license-server"
echo "================================================================"
