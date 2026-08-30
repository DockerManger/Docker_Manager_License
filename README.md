# Docker Manager License

> [Docker_Manager_Go](https://github.com/DockOrae/Docker_Manager_Go) 的官方 License 签发、管理与授权服务端。
>
> 更新说明见 [CHANGELOG.md](CHANGELOG.md)。

```
DockOrae-Auth          Docker_Manager_Go (开源)
┌─────────────────────┐         ┌─────────────────────┐
│ Admin Panel (Vue3)  │         │ Ed25519 Public Key  │
│ License API (Gin)   │◄─激活───│ License Verify      │
│ 在线验证/设备绑定    │◄─验证───│ Expiration          │
│ PostgreSQL          │◄─解绑───│ Features            │
│ Ed25519 Private Key │         └─────────────────────┘
└─────────────────────┘
```

**核心原则：签发权属于 License Server，验证权属于 Docker_Manager_Go。
私钥永远留在 License Server；Docker_Manager_Go 源码公开，只持有公钥。
在线授权闭环：本地 Ed25519 验签防伪造 + 在线激活/设备绑定 + 定期验证(24h)+ 吊销即时生效。**

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
| `web/` | Vue 3 + TypeScript 管理后台(shadcn-vue,14 语言 i18n) |

## 管理后台 UI

- **登录页背景图**:内置壁纸(`web/public/bg.jpg`,与 Docker_Manager_Go 同一张),1Panel 风格
  背景图 + 深色遮罩,暗色/亮色主题下均适配;图片加载失败自动回退纯色背景
- **右上角工具栏**:主题切换(明/暗,圆形扩散过渡)+ 语言切换(14 语言下拉,含 RTL),
  与 Docker_Manager_Go 登录页交互一致;登录后主界面右上角同样提供主题与语言切换
- **多语言**:14 种语言(zh-CN / zh-TW / en / ja / ko / ru / vi / es / id / uk / tr /
  pt-BR / ar / fa),浏览器语言自动检测,选择持久化
- **深色/浅色主题**:Design Tokens 驱动,默认深色,切换带圆形扩散过渡动画

## 安全设计

- **Ed25519 非对称签名**：私钥只在签发端,消费端只持公钥 —— 源码公开也无法伪造 License
- **私钥隔离**：`.gitignore` 强制排除 `*.key/*.pem/secrets/`；Dockerfile 只挂载卷,绝不 COPY；CI 发布前自动扫描密钥文件
- **Key 轮换预留**：每个 License 携带 `key_id`(如 `2026-01`),未来可多密钥共存
- **JWT token_version**：修改密码 / 吊销 token 后立即失效,不重蹈"改密码旧 token 仍有效"的覆辙
- **统一错误结构**：`{"error":{"code","message"}}`,不泄露 SQL/路径/堆栈
- **审计日志**：签发/延期/吊销/登录全部记录(操作人/IP/详情)

## License 格式 (V2.1)

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
  "customer_id": "CUS-01JXXXXXXXXXXXX",      // V2.1 新增(可选,关联 customers 表)
  "subscription_id": "SUB-01JXXXXXXXXXXXX",  // V2.1 新增(可选,关联 subscriptions 表)
  "issued_at": 1777392000,
  "expires_at": 1808928000,
  "max_devices": 3
}
```

> **V1(HMAC)已完全移除**(2026-08):只签发/验证 V2 Ed25519。
> **V2.1 向后兼容**:新增字段全部可选,存量 V2 Key 继续有效;`version` 严格检查,未知版本拒绝。

机器可读契约:`docs/license-schema.json`(payload)+ `docs/openapi.yaml`(API)。

完整契约与 Docker_Manager_Go 改造清单见 **[docs/integration.md](docs/integration.md)**。

## 快速开始 (Docker Compose,单容器全包)

> **完整的「一步一步照着做」部署流程(IP 直连 / 域名 HTTPS 两种方式)见 [docs/DEPLOY.md](docs/DEPLOY.md)。**
> 下面是最简版本。

> 镜像内置 license-server + nginx 反代 + PostgreSQL,**对外仅暴露 80 端口**,
> 无需在宿主机配置 nginx / PostgreSQL / .env。

### 一键部署(推荐)

```bash
curl -fsSL https://github.com/DockOrae/DockOrae-Auth/raw/refs/heads/master/deploy/install.sh | bash
```

脚本自动:检测环境 → 创建部署目录(`~/dockorae-auth`)→ 下载 compose → 拉镜像 → 启动 → 打印管理员账号/密码/公钥。

可选参数(环境变量):
```bash
DML_DIR=~/dml DML_PORT=8080 curl -fsSL https://github.com/DockOrae/DockOrae-Auth/raw/refs/heads/master/deploy/install.sh | bash
# 国内网络加速: DML_MIRROR=https://docker.m.daocloud.io ...
# 重新部署: DML_FORCE=1 ...
```

### 手动部署(等价,一条命令)

```bash
# 无需 .env —— 所有敏感配置首次启动自动生成并打印在日志里
docker compose up -d
docker compose logs -f license-server
```

### 域名接入(推荐,Docker_Manager_Go 已内置固定地址)

Docker_Manager_Go 内置官方授权服务器地址 `https://manager.kejizero.xyz/license-api`,
**nginx 反代已内置在镜像中**,你只需要:

1. 在 Cloudflare 把域名 A 记录指向服务器 IP(开启 Proxied 橙色云朵 → 免费 HTTPS)
2. 完成,无需任何 nginx 配置

客户端实际请求: `https://manager.kejizero.xyz/license-api/api/v1/public/activate|verify|deactivate`
(HTTPS 必需;开发/自建服务器可用环境变量 `DM_LICENSE_SERVER_URL` 覆盖客户端地址)。

首次启动自动完成:
1. 初始化内置 PostgreSQL(migration 自动执行,数据在 `./pgdata`)
2. 生成 Ed25519 私钥 → `./private/license.key` (0600,entrypoint 自动修复目录权限)
3. 生成 JWT secret → `./data/jwt_secret` (重启不失效)
4. 创建管理员,日志中打印:

```
==============================================
初始管理员已创建,请立即登录并修改密码:
  地址: http://<服务器IP>:80
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

| 表: `admins` / `customers`(客户,V3) / `subscriptions`(订阅,V3) / `licenses` / `license_revisions`(修订历史,不覆盖) / `activations`(设备激活,在线闭环) / `activation_tokens`(激活凭据,**只存 SHA-256**) / `security_events`(安全事件) / `security_nonces`(重放防护) / `server_settings`(minimum_client_version / blocked_versions) / `audit_logs` / `signing_keys`(密钥注册表)

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
| GET | `/api/v1/admin/licenses/:id/activations` | ✅ | 设备激活列表 |
| POST | `/api/v1/admin/licenses/:id/activations/:aid/deactivate` | ✅ | 单个设备解绑 |
| POST | `/api/v1/admin/licenses/:id/reset-devices` | ✅ | 重置全部设备(审计) |
| GET | `/api/v1/admin/signing-keys` | ✅ | 签名密钥注册表 |
| GET | `/api/v1/admin/audit-logs` | ✅ | 审计日志 |
| POST | `/api/v1/admin/customers` | ✅ | 创建客户(CUS-*) |
| GET | `/api/v1/admin/customers` | ✅ | 客户列表 |
| POST | `/api/v1/admin/subscriptions` | ✅ | 创建订阅(SUB-*) |
| GET | `/api/v1/admin/subscriptions` | ✅ | 订阅列表 |
| POST | `/api/v1/admin/subscriptions/:id/status` | ✅ | 更新订阅状态 |
| GET | `/api/v1/admin/security-events` | ✅ | 安全事件列表(按类型过滤) |
| GET/PUT | `/api/v1/admin/settings` | ✅ | 服务器配置(minimum_client_version / blocked_versions) |
| POST | `/api/v1/public/activate` | - | 在线激活(传完整 Key + 设备信息,限流) |
| POST | `/api/v1/public/verify` | - | 定期验证(**activation_token + timestamp + nonce**,不携带 Key) |
| POST | `/api/v1/public/deactivate` | - | 客户端解绑(activation_token + device_id) |

## 在线授权闭环(客户端契约,V3)

Docker_Manager_Go 接入流程(完整契约见 `docs/integration.md` §8):

```
导入 License → 本地 Ed25519 验签 → POST /public/activate {key, device_id, ...}
  → 返回 activation_id + activation_token(明文仅此一次)+ server_time + next_verify_after
→ 每 24h POST /public/verify {activation_token, device_id, timestamp, nonce}
  → valid:继续 Pro / blocked:版本封禁 / revoked|expired|invalid:禁用 Pro
→ 解绑 POST /public/deactivate {activation_token, device_id, timestamp, nonce}
```

- **License Key 只用于首次激活/重新激活**(Skill §13);运行期验证用 Activation Token
- **Token 不明文存库**:数据库只存 `SHA-256(token)`,客户端本地 `license.json` 权限 0600
- **重放防护**:timestamp ±5 分钟窗口 + nonce 一次性(SHA-256 存储,定期清理)
- **Server Time**:所有响应带 `server_time`,客户端维护 `clock_offset`(trusted_now),防本地时间作弊;
  本地时钟回退 > 5 分钟 → `CLOCK_ROLLBACK_DETECTED` 禁用 Pro
- **版本控制**:服务端可设置 `minimum_client_version`(客户端提示升级)与
  `blocked_versions`(封禁版本 → verify 返回 blocked,禁用 Pro)
- **设备上限**:服务端事务 + 行锁保证并发激活不突破 `max_devices`(有并发测试验证)
- **防跨设备解绑**:deactivate/verify 必须凭据(token)匹配 License + 设备
- **吊销即时生效**:客户端下次 verify 即收到 `revoked` 并禁用 Pro;吊销时服务端立即吊销全部激活 token
- **限流**:activate/deactivate 15min/20 次,verify 15min/120 次(IP 级,防 Key 爆破)
- **安全事件**:无效签名/token、重放、限流超限、版本封禁等写入 `security_events`(管理端可查;
  绝不记录 key/token/私钥)
- **Grace Period 由客户端本地维护**(`last_successful_verify` + 7 天宽限,服务端不强制;
  revoked/blocked 不能进入宽限)

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

覆盖:签名/篡改/过期/吊销/延期/修订/权限/限流/密钥不泄露/消费端验证(见 `internal/integration`)、
V3 token 验证/重放防护/安全事件/版本控制/客户订阅(见 `internal/api/api_v3_test.go`)。

## 部署注意事项

- License Server 是**私有的授权签发端**,不建议公开暴露管理 API(内网/VPN 部署)
- 在线验证是授权闭环的核心:客户端接入 `activate/verify/deactivate` 后,吊销/过期即时生效;
  纯离线使用(不接入在线验证)仍可本地验签运行,但无法感知吊销(产品文档需明确此差异)
- 数据库(PostgreSQL)数据务必定期备份(激活记录/审计日志不可重建)
