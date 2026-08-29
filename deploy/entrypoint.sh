#!/bin/sh
# 容器入口:修复挂载目录属主后,降权为 license 用户运行
# 解决"宿主目录 root 属主导致容器内无法写私钥"的权限问题(无需手动 chown)
set -e

# 修复数据/密钥目录属主(挂载卷可能由宿主 root 创建)
chown -R license:license /private /data 2>/dev/null || true

# 降权执行(license 用户 uid:gid = 100:100)
exec su-exec license "$@"
