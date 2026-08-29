# Docker Manager License

> [Docker_Manager_Go](https://github.com/MinimaxFlora/Docker_Manager_Go) 的官方 License 签发、管理与授权服务端。

```
Docker_Manager_License          Docker_Manager_Go (开源)
┌─────────────────────┐         ┌─────────────────────┐
│ Admin Panel (Vue3)  │         │ Ed25519 Public Key  │
│ License API (Gin)   │         │ License Verify      │
│ PostgreSQL          │         │ Expiration          │
│ Ed25519 Private Key │         │ Features            │
└─────────┬───────────┘         └─────────┬───────────┘
          │   Signed License (离线文件)     │
          └────────────────────────────────┘
```

**核心原则：签发权属于 License Server，验证权属于 Docker_Manager_Go。
私钥永远留在 License Server；Docker_Manager_Go 源码公开，只持有公钥。**

---

## 架构

| 模块 | 说明 |
|---|---|
| `cmd/license-server` | 服务入口(含 `migrate` / `keygen` 子命令) |
| `internal/license` | **License V2 格式契约**(两端共享,见 `docs/integration.md`) |
| `internal/crypto` | Ed25519 密钥管理(私钥 0600 文件,永不进代码/镜像) |
| `internal/auth` | 管理员认证:argon2id + JWT(token_version) + TOTP 2FA + 登录限流 |
| `internal/service` | 业务层:签发/延期/吊销/修订/审计 |
| `internal/api` | Gin 路由(公开 API 与管理 API 物理分离) |
| `internal/database` | PostgreSQL + 版本化 migration(`internal/database/migrations/`) |
| `web/` | Vue 3 + TypeScript 管理后台 |

## 安全设计

- **Ed25519 非对称签名**：私钥只在签发端,消费端只持公钥 —— 源码公开也无法伪造 License
- **私钥隔离**：`.gitignore` 强制排除 `*.key/*.pem/secrets/`；Dockerfile 只挂载卷,绝不 COPY；CI 发布前自动扫描密钥文件
- **Key 轮换预留**：每个 License 携带 `key_id`(如 `2026-01`),未来可多密钥共存
- **JWT token_version**：修改密码 / 吊销 token 后立即失效,不重蹈"改密码旧 token 仍有效"的覆辙
- **统一错误结构**：`{"error":{"code","message"}}`,不泄露 SQL/路径/堆栈
- **审计日志**：签发/延期/吊销/登录全部记录(操作人/IP/详情)

## License 格式 (V2)

```
Key = base64url(规范JSON payload) + "." + base64url(Ed25519签名64字节)

payload = {
  "version": 2,
  "key_id": "2026-01",
  "license_id": "DMG-01JXXXXXXXXXXXX",
  "product": "docker-manager-go",
  "plan": "pro",
  "features": ["compose", "container_create", "appstore"],
  "customer": "Zhao",
  "issued_at": 1777392000,
  "expires_at": 1808928000,
  "max_devices": 3
}
```

完整契约与 Docker_Manager_Go 改造清单见 **[docs/integration.md](docs/integration.md)**。

## 快速开始 (Docker Compose)

```bash
# 无需 .env —— 所有敏感配置首次启动自动生成并打印在日志里
docker compose -f deploy/docker-compose.yml up -d
docker compose -f deploy/docker-compose.yml logs -f license-server
```

首次启动自动完成:
1. 初始化 PostgreSQL(migration 自动执行)
2. 生成 Ed25519 私钥 → `./private/license.key` (0600,entrypoint 自动修复目录权限)
3. 生成 JWT secret → `./data/jwt_secret` (重启不失效)
4. 创建管理员,日志中打印:

```
==============================================
初始管理员已创建,请立即登录并修改密码:
  地址: http://<服务器IP>:3000
  用户名: admin
  密码: <自动生成的随机密码>
==============================================
```

日志还会打印 **PUBLIC KEY**(PEM)——复制保存,它是 Docker_Manager_Go 集成所需的唯一密钥材料。

可选:自定义数据库密码/管理员密码 → 设置环境变量后启动(`POSTGRES_PASSWORD` / `ADMIN_USERNAME` / `ADMIN_PASSWORD`)。

## 本地开发

```bash
# 依赖
go mod tidy
cd web && npm install

# 数据库(任意 PostgreSQL 实例)
export DATABASE_URL=postgres://license:licensepass@localhost:5432/license?sslmode=disable

# 运行
make run          # 完整构建(web + backend)并启动 :3000

# 或分开
make web          # 前端 → web/dist
make backend      # 后端 → license-server
```

## 环境变量

| 变量 | 必填 | 说明 |
|---|---|---|
| `DATABASE_URL` | ✅ | PostgreSQL DSN |
| `JWT_SECRET` | ✅ | JWT 签名密钥(≥32 字符随机) |
| `LICENSE_KEY_ID` | ✅ | 当前签发密钥标识(如 `2026-01`) |
| `LICENSE_PRIVATE_KEY_PATH` | | 私钥路径,默认 `private/license.key` |
| `SERVER_ADDR` | | 监听地址,默认 `:3000` |
| `JWT_TTL` | | JWT 有效期,默认 `12h` |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | 首次 | 初始化管理员(仅首次生效) |

## 数据库

所有 Schema 变更走版本化 migration(`internal/database/migrations/`),启动自动执行,
`make migrate` 可单独执行。禁止运行时"猜结构"改表。

表: `admins` / `licenses` / `license_revisions`(修订历史,不覆盖) / `activations`(设备预留) / `audit_logs`

## 密钥管理

```bash
make keygen    # 生成 private/license.key + private/license.pub
```

- 私钥只存在于文件系统(0600),通过 volume 挂载进容器
- **密钥泄露处理**：生成新密钥对 → 新 `key_id` → 发布新公钥 → 旧 key 标记 revoked → 重新签发。因为 License 携带 `key_id`,消费端可以区分新旧公钥
- 生产环境未来可升级 KMS/Vault(第一版刻意不做,保持简单)

## API

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| POST | `/api/v1/admin/login` | - | 登录(密码 + 可选 TOTP) |
| POST | `/api/v1/admin/logout` | ✅ | 登出(`?revoke_all=true` 吊销全部会话) |
| GET | `/api/v1/admin/me` | ✅ | 当前用户 |
| POST | `/api/v1/admin/change-password` | ✅ | 改密码(旧 token 全部失效) |
| POST | `/api/v1/admin/setup-totp` / `confirm-totp` / `disable-totp` | ✅ | 2FA 管理 |
| GET | `/api/v1/admin/stats` | ✅ | 概览统计 |
| POST | `/api/v1/admin/licenses` | ✅ | 签发 |
| GET | `/api/v1/admin/licenses` | ✅ | 列表(分页+状态筛选) |
| GET | `/api/v1/admin/licenses/:id` | ✅ | 详情 |
| GET | `/api/v1/admin/licenses/:id/revisions` | ✅ | 修订历史 |
| GET | `/api/v1/admin/licenses/:id/export` | ✅ | 导出 Key(.lic 下载) |
| POST | `/api/v1/admin/licenses/:id/extend` | ✅ | 延期(新修订) |
| POST | `/api/v1/admin/licenses/:id/revoke` | ✅ | 吊销(软删除) |
| GET | `/api/v1/admin/audit-logs` | ✅ | 审计日志 |
| POST | `/api/v1/public/verify` | - | 在线状态查询(不替代本地验证) |

## License 生命周期

```
Created → Active ──→ Expired
            │
            └──→ Revoked (保留记录与修订,不物理删除)
```

## 测试

```bash
make test                 # gofmt + vet + test + race(与 CI 一致)
go test ./internal/...    # 单元测试(本地可跑)
go test ./internal/api/   # DB 集成测试(需 TEST_DATABASE_URL,CI 提供)
```

覆盖:签名/篡改/过期/吊销/延期/修订/权限/限流/密钥不泄露/消费端验证(见 `internal/integration`)。

## 部署注意事项

- License Server 是**私有的授权签发端**,不建议公开暴露管理 API(内网/VPN 部署)
- 离线 License 无法实时感知吊销 —— 产品文档需明确:吊销仅对**在线验证**场景即时生效
- 数据库(PostgreSQL)数据务必定期备份
