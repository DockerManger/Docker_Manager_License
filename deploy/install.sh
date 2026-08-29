#!/bin/bash
# ============================================================
# Docker Manager License — 一键部署脚本(Compose 单容器全包)
#
# 一条命令部署(仓库已公开):
#   curl -fsSL https://github.com/DockerManger/Docker_Manager_License/raw/refs/heads/master/deploy/install.sh | bash
#
# 等价于:
#   docker compose up -d
#
# 镜像内置 license-server + nginx 反代 + PostgreSQL,对外仅暴露 80 端口。
# 部署后只需:
#   1. DNS 把域名 A 记录指向本机 IP(Cloudflare 开 Proxied 即免费 HTTPS)
#   2. 客户端走 https://<域名>/license-api/...
# 无需在宿主机配置 nginx / PostgreSQL!
#
# 特性:
#   - 自动检测 Docker / Compose 环境
#   - 重复执行自动检测已部署状态并显示服务状态(不重复初始化)
#   - 部署完成后打印:访问地址 / 管理员账号 / 初始密码 / 公钥
#
# 参数(通过环境变量):
#   DML_DIR=~/dml         部署目录(默认 ~/docker-manager-license)
#   DML_PORT=8080          对外端口(默认 80)
#   DML_FORCE=1            强制重新部署
#   DML_MIRROR=<registry>  镜像加速源(如 https://docker.m.daocloud.io)
# ============================================================
set -e

# ---------- 配置 ----------
INSTALL_DIR="${DML_DIR:-$HOME/docker-manager-license}"
PORT="${DML_PORT:-80}"
FORCE="${DML_FORCE:-0}"
MIRROR="${DML_MIRROR:-}"
IMAGE="zhaoweiwen123/docker_manager_license:latest"
# 仓库已公开,compose 直接从 GitHub 拉取;优先 github.com raw 端点(绕过 raw CDN 滞后)
GH_RAW="https://github.com/DockerManger/Docker_Manager_License/raw/refs/heads/master/deploy/docker-compose.yml"
COMPOSE_URLS=(
  "${DML_MIRROR:+$DML_MIRROR/}$GH_RAW"
  "$GH_RAW"
  "https://raw.githubusercontent.com/DockerManger/Docker_Manager_License/master/deploy/docker-compose.yml"
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

# 端口自定义(默认 80,映射到容器内 80)
if [ "$PORT" != "80" ]; then
  sed -i "s/\"80:80\"/\"$PORT:80\"/" docker-compose.yml
  c "端口已改为 $PORT"
fi

# ---------- 5. 拉取镜像(可选加速源) ----------
c "==> 拉取镜像 $IMAGE ..."
if [ -n "$MIRROR" ]; then
  MIRROR_IMG="${MIRROR%/}/zhaoweiwen123/docker_manager_license:latest"
  warn "使用加速源 $MIRROR_IMG"
  docker pull "$MIRROR_IMG" || { warn "加速源拉取失败,改用官方源"; docker pull "$IMAGE"; }
  docker tag "$MIRROR_IMG" "$IMAGE" 2>/dev/null || true
else
  docker pull "$IMAGE"
fi
ok "镜像就绪"

# ---------- 6. 启动 ----------
c "==> 启动容器(首次约 1-2 分钟,自动初始化数据库 + 生成私钥/管理员)..."
docker compose up -d

# ---------- 7. 输出部署信息 ----------
sleep 6
echo ""
echo "=================================================================="
c "  Docker Manager License 部署完成"
c "  管理后台: http://<服务器IP>:$PORT"
c "  公开 API: http://<服务器IP>:$PORT/license-api/ (配置域名后走 HTTPS)"
echo "------------------------------------------------------------------"
c "  下一步:在 Cloudflare 把域名 A 记录指向本机 IP(开 Proxied 免费 HTTPS),"
c "  Docker_Manager_Go 内置地址即 https://manager.kejizero.xyz/license-api"
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
