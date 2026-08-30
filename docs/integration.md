# Docker_Manager_Go 集成文档(License V2.1 / V3 架构契约)

> 本文档是 **DockOrae-Auth(签发端)** 与 **Docker_Manager_Go(消费端)** 之间唯一的正式契约。
> 任何一方的修改必须同步另一方,否则授权失效。本仓库 `internal/integration/consumer_test.go`
> 用独立实现模拟了消费端验证逻辑,改造 Docker_Manager_Go 时以此为准。
>
> 机器可读契约:`docs/license-schema.json`(payload 校验);`docs/openapi.yaml`(API 描述)。

---

## 1. 数据流

```
DockOrae-Auth                    Docker_Manager_Go (开源)
        │                                        │
   Ed25519 私钥                                 Ed25519 公钥(内嵌,key_id 注册表)
        │                                        │
   签发 Key 字符串                              Parse → Verify → Expiry → Feature
        │                                        │
        └────── License Key(仅首次激活)─────────┘
        └────── Activation Token(运行期验证)─────┘
                 (token 数据库只存 SHA-256 hash)
```

**安全模型**:
- 攻击者拿到 Docker_Manager_Go 全部源码 → 只能看到公钥,**不能生成合法 License**
- 攻击者拿到 License 文件 → 不能修改有效期/features/max_devices(签名失效)
- 攻击者复制 License → 只能激活 max_devices 台设备(服务端权威计数)
- **V1(HMAC)已完全移除**:不再保留 V1/V2 双轨,消费端只接受 V2 Ed25519;
  存量 V1 Key 无法激活,需重新签发

---

## 1.5 对外 API Base URL(唯一规范)

生产环境固定唯一 Base URL(禁止多套前缀):

```
https://manager.kejizero.xyz/license-api
```

客户端请求 = **Base URL + 内部 API 路径**:

| 操作 | 完整 URL(对外) | 内部路径(license-server) |
|---|---|---|
| 激活 | `https://manager.kejizero.xyz/license-api/api/v3/activate` | `/api/v3/activate` |
| 验证 | `https://manager.kejizero.xyz/license-api/api/v3/verify` | `/api/v3/verify` |
| 解绑 | `https://manager.kejizero.xyz/license-api/api/v3/deactivate` | `/api/v3/deactivate` |
| SSE 事件流 | `https://manager.kejizero.xyz/license-api/api/v3/events` | `/api/v3/events`(长连接) |
| 健康检查 | `https://manager.kejizero.xyz/license-api/health` | `/health`(别名 `/healthz` 兼容) |
| 管理后台 | `https://manager.kejizero.xyz/`(SPA) | 前端 + `/api/v1/admin/*` |

**前缀剥离**:反代配置 `uri strip_prefix /license-api` 后转发到 license-server 容器,
license-server 内部**没有** `/license-api` 前缀——所有内部路由以 `/api/...` 开头。
**V1/V2 公开 API(`/api/v1/public`)已删除**,只保留 `/api/v3`(公开)与 `/api/v1/admin`(管理)。

开发/私有部署可用环境变量 `DM_LICENSE_SERVER_URL` 覆盖客户端地址;
License Server 侧 `SERVER_ADDR` 控制监听(默认 `:3000`)。

---

## 2. Key 字符串格式

```
<base64url(规范JSON payload)>.<base64url(Ed25519签名 64 字节)>
```

示例(结构):
```
eyJ2ZX...MDAw...}.<88位签名>
```

**V1 已彻底删除**:V1 HMAC 验证路径、`licenseSecret`、`LicenseSign` 已从 Docker_Manager_Go
移除,不保留 V1/V2 双协议。消费端只接受 V2(Ed25519)。存量 V1 Key 无法激活,需重新签发 V2 License。

---

## 3. Payload 规范(V2.1)

字段顺序即规范 JSON 序列化顺序(Go struct 字段序),**签名覆盖完整 payload 字节**。

```json
{
  "version": 2,
  "key_id": "2026-01",
  "license_id": "DMG-01JXXXXXXXXXXXX",
  "product": "docker-manager-go",
  "plan": "pro",
  "features": ["compose", "container_create", "appstore"],
  "customer": "Zhao",
  "customer_id": "CUS-01JXXXXXXXXXXXX",
  "subscription_id": "SUB-01JXXXXXXXXXXXX",
  "issued_at": 1777392000,
  "expires_at": 1808928000,
  "max_devices": 3
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | int | 必须为 `2`;遇到更高版本必须拒绝(`UNSUPPORTED_LICENSE_VERSION`),不得静默接受 |
| `key_id` | string | 签发密钥标识(轮换/泄露处理用),如 `2026-01`;未知 key_id 拒绝(`UNSUPPORTED_KEY`) |
| `license_id` | string | 全局唯一展示 ID(客服查询凭据),前缀 `DMG-` |
| `product` | string | 固定 `docker-manager-go`,不符直接拒绝 |
| `plan` | string | `pro`(当前唯一可签发套餐;`free` 不需要 License;business/enterprise 预留) |
| `features` | string[] | Feature Registry 子集(见 §4);未知 feature 拒绝 |
| `customer` | string | 客户名(展示用) |
| `customer_id` | string | **V2.1 新增(可选)**:customers 表 `CUS-*` 关联。存量 V2 Key 无此字段仍有效 |
| `subscription_id` | string | **V2.1 新增(可选)**:subscriptions 表 `SUB-*` 关联。存量 V2 Key 无此字段仍有效 |
| `issued_at` / `expires_at` | int64 | Unix 秒;`expires_at < now` → expired;`issued_at > expires_at` 非法 |
| `max_devices` | int | 允许绑定设备数(>=1) |

> **兼容性承诺**:V2.1 只新增可选字段,不改变已有字段语义与签名顺序。
> 消费端 JSON 解析天然忽略未知字段;存量 V2 Key(无 customer_id/subscription_id)继续有效。

---

## 4. Feature Registry(两端必须逐字一致)

| Feature | Docker_Manager_Go 门控点 |
|---|---|
| `compose` | `internal/service/compose.go` 部署入口(`license.required` 403) |
| `container_create` | `internal/service/container.go` 创建入口 |
| `appstore` | `internal/service/appstore.go` 安装入口 |

> ⚠️ 严禁两端各自起名(如 `advanced-compose` vs `compose_advanced`)。
> 新增 Feature 必须先改本 Registry + 本仓库 `internal/license/format.go`(FeatureRegistry),
> 再同步消费端。规划中(未注册,不可签发):`terminal` / `backup` / `monitor` /
> `multi_node` / `api` / `automation`。

## 4.1 Plan Registry(统一套餐注册表)

| Plan | Features | 状态 |
|---|---|---|
| `free` | 无(不需要 License) | 不可签发 |
| `pro` | compose + container_create + appstore | ✅ 可签发 |
| `business` | pro + multi_node + api | 预留(feature 落地后启用) |
| `enterprise` | business + audit + sso | 预留 |

套餐逻辑集中在 `internal/license/format.go` 的 `PlanRegistry`,严禁散落在业务代码。
签发校验:`plan ∈ EnabledPlanNames` 且 `features ⊆ FeatureRegistry`。

---

## 5. Docker_Manager_Go 改造清单(V3)

基于 V3 架构(2026-08),消费端已实现:

### 5.1 `internal/service/license.go`

- 公钥注册表 `licensePublicKeys`(key_id → PEM);未知 key_id 拒绝
- `LicenseVerifyKey` 唯一 V2 路径(无 V1 分派);严格校验:
  version==2 / product / plan / features ⊆ Registry / issued_at / expires_at / max_devices
- `LicenseFeatureActive(st, feature)` 功能级门控(空 features = 无商业功能)
- `LicenseDeviceFingerprint(dataDir)` 设备指纹 SHA-256(稳定机器信息,不含 MAC)

### 5.2 `internal/service/license_verify.go`(V3 在线闭环)

- license.json 存储:`key / device_id / activation_id / activation_token / last_successful_verify /
  verify_state / server_url / last_server_time / last_local_time / clock_offset`
- verify 请求:仅 `activation_token + device_id + product_version + timestamp + nonce`(**不再携带完整 Key**)
- 服务端 `server_time` → 本地 `clock_offset` → `trusted_now`(防本地时间作弊)
- 本地时钟回退检测:回退 > 5 分钟 → `CLOCK_ROLLBACK_DETECTED`(禁用 Pro)
- `minimum_client_version` 高于当前版本 → `UPDATE_REQUIRED`(提示,不封禁);
  `blocked_versions` 命中 → `CLIENT_VERSION_BLOCKED`(禁用 Pro)
- license.json 权限 `0600`;`activation_token` 绝不写入日志
- 兼容:旧服务端(未升级)返回 `key is required` 时自动回退旧格式(key + activation_id)

### 5.3 门控点

```go
license: func() bool { return service.LicenseFeatureActive(st, "compose") }
```

---

## 6. 部署步骤(接入正式环境)

1. 部署 License Server(见 README),首次启动记录输出的 **PUBLIC KEY**
2. 把公钥写入 `internal/service/license.go` 的 `licensePublicKeys` 映射(key_id 与 Server 的 `LICENSE_KEY_ID` 一致)
3. 升级顺序:**先升级 License Server**(003 migration 自动执行),再发布 Docker_Manager_Go
   (旧客户端在升级窗口期通过兼容路径继续工作)
4. 用 License Server 签发一个测试 License → 在面板粘贴激活 → 验证 Pro 生效
5. 存量激活设备:003 migration 已把旧 activation_code 迁移为 token hash,
   客户端升级后本地旧凭据(activation_id=旧 code)仍可完成 verify/deactivate,**无需重新激活**

## 7. 兼容性策略

- **V1 已彻底删除**(2026-08):客户端只验证 V2 Ed25519,License Server 只签发 V2
- **V2.1 向后兼容**:新增可选字段(customer_id/subscription_id),存量 V2 Key 继续有效
- **升级窗口兼容**:服务端 verify/deactivate 保留旧格式(key + activation_id)路径
  (文档标注 deprecated);新客户端对旧服务端自动回退
- License payload 带 `version`,未来 v3 由消费端明确拒绝而不是静默接受

## 8. 在线授权闭环 API(客户端接入,V3 Event-Driven)

客户端(消费端)导入 License 并通过本地 Ed25519 验签后,进入在线闭环:

> **同步模型(V3)**:License 状态变化由 Server 主动通过 **Event + SSE** 通知客户端,
> 客户端收到事件后调用 Verify 获取权威状态。**Event = Trigger,Verify = Authority**。
> 禁止任何周期性 Verify / Heartbeat / Check-in / Lease 机制。
> V2/Legacy 兼容路径(旧格式 key + activation_id)已全部删除,不存在 V3→V2 fallback。

### 8.1 激活(携带完整 License Key,仅首次激活/重新激活用)

```
POST /api/v3/activate
{
  "key": "<完整 License Key>",
  "device_id": "<机器唯一ID>",
  "device_name": "可选",
  "product_version": "可选",
  "device_fingerprint": "可选(SHA-256 稳定机器信息)",
  "platform": "可选(linux/windows/...)",
  "architecture": "可选(amd64/arm64/...)"
}
```

成功(200):
```json
{
  "status": "active",
  "activation_id": "ACT-01JXXXXXXXXXXXX",
  "activation_token": "<64位hex,明文只返回这一次>",
  "license_id": "DMG-...",
  "expires_at": 1810000000,
  "features": ["compose"],
  "max_devices": 3,
  "server_time": 1780000000,
  "state_version": 1
}
```

失败(统一错误体):`INVALID_SIGNATURE` / `LICENSE_NOT_FOUND` / `LICENSE_REVOKED` /
`LICENSE_EXPIRED` / `DEVICE_LIMIT_REACHED` / `RATE_LIMITED`。

语义:
- 同一设备重复激活 → 幂等(200,同一 activation_id,签发新 token,旧 token 吊销;`state_version+1`)
- 解绑过的设备重新激活 → 恢复 active,发新 activation_id + 新 token,不占新额度
- 活跃设备数 >= max_devices → `DEVICE_LIMIT_REACHED`(服务端事务+行锁,并发激活不突破)
- 激活成功同时持久化 `activation.created` / `activation.rebound` 事件并推送 SSE

### 8.2 Verify(权威状态查询;仅由事件/重连/启动/手动触发,**无周期验证**)

```
POST /api/v3/verify
{
  "activation_token": "...",
  "device_id": "...",
  "product_version": "可选",
  "timestamp": 1780000000,
  "nonce": "<32字节随机hex>"
}
```

返回:
```json
{
  "status": "valid",
  "valid": true,
  "license_id": "DMG-...",
  "plan": "pro",
  "customer": "Zhao",
  "expires_at": 1810000000,
  "features": ["compose"],
  "server_time": 1780000000,
  "state_version": 2,
  "minimum_client_version": "1.5.0"
}
```

| status | 客户端动作 |
|---|---|
| `valid` | 继续 Pro,记录 `last_successful_verify` + `server_time`(更新 clock_offset)+ `state_version` |
| `blocked` | 版本被封禁,立即禁用 Pro(`CLIENT_VERSION_BLOCKED`) |
| `revoked` | 立即禁用 Pro,提示"License revoked" |
| `expired` | 立即禁用 Pro(本地 expires_at 也应同时判断) |
| `invalid` | 设备未激活/凭据不匹配,禁用 Pro |

**版本控制**(服务端 `server_settings`):
- `minimum_client_version`:客户端当前版本低于该值 → 本地标记 `UPDATE_REQUIRED`(提示升级,不封禁)
- `blocked_versions`(JSON 数组):当前版本命中 → 服务端返回 `status=blocked`,禁用 Pro
  (用于严重安全漏洞/协议漏洞/绕过漏洞的紧急封禁);策略变更时推送全局事件 `version_policy.changed`

**重放防护**:`timestamp` 必须落在服务端时间 ±5 分钟窗口内,`nonce` 必须未使用
(服务端存 SHA-256,1 小时后清理)。违规返回 `400 REPLAY_DETECTED`。

### 8.3 解绑

```
POST /api/v3/deactivate
{
  "activation_token": "...",
  "device_id": "...",
  "timestamp": 1780000000,
  "nonce": "..."
}
```

- 凭据(token)必须匹配该 License + 设备,防止 Device A 解绑 Device B(不匹配 → `ACTIVATION_NOT_FOUND`)
- 吊销/过期的 License 也允许解绑(客户端清理)
- 服务端同时吊销该激活的全部 token(解绑后旧 token 立即失效),并推送 `activation.unbound` 事件
  (payload `{"source": "user" | "admin", "reason": "user_unbound" | "admin_unbound"}`,
  客户端据此区分"用户主动解绑"与"管理员强制解绑"的提示文案)
- **解绑 ≠ 吊销**:解绑只把激活置为 `deactivated`,License 保持 `ACTIVE`,可随时重新激活
- **解绑后 verify 返回 `unbound`**:旧 token 调用 verify 时,服务端定位原激活记录,
  若 License 仍 ACTIVE 且激活为 deactivated → 返回 `status=unbound`(携带 license_id/expires_at/features),
  客户端清除本地凭据、保留 Key,提示"请重新激活许可证";绝不显示"已吊销"
- **吊销后 verify 返回 `revoked`**:License 或激活已 REVOKED → `status=revoked`,
  客户端显示"许可证已被吊销"(唯一允许显示"已吊销"的场景)

### 8.4 SSE 事件流(主动同步核心)

```
GET /api/v3/events
Authorization: Bearer <activation_token>
X-Device-ID: <device_id>
Last-Event-ID: evt_<sequence>(可选,标准 SSE 头)
```

- 长连接;Server 状态变化时推送事件(Event Store 持久化,`license_events` 表)
- 事件只是 Trigger:客户端收到后必须调用 `/api/v3/verify` 获取权威状态,不直接改授权结论
- **Replay**:重连携带 `Last-Event-ID`,服务端补齐缺失事件;无法补齐 → 推送 `resync_required` → 客户端 Verify
- **Gap 检测**:事件序号跳变(中间被清理)→ `resync_required`
- 事件格式:`event: <type>` / `id: evt_N` / `data: {...}`,20s 保活注释行(`: keep-alive`,非 Heartbeat)
- 事件类型:`license.changed` / `license.revoked` / `activation.created` / `activation.revoked` /
  `activation.unbound` / `activation.rebound` / `version_policy.changed` 等
- SSE 认证绑定激活:Device A 只能收到 Device A 的事件(凭据无效 → 401)

### 8.5 限流

`activate`/`deactivate`:15min 20 次/IP;`verify`:15min 120 次/IP。超限 `RATE_LIMITED`。
超限与无效凭据均写入 `security_events`(管理端可查)。

### 8.6 Server Time(防本地时间作弊)

- 所有公开 API 成功响应携带 `server_time`
- 客户端保存 `last_server_time / last_local_time`,计算 `clock_offset = server_time - local_time`
- 之后使用 `trusted_now = local_now + clock_offset` 判断过期
- 本地时钟回退检测:`local_now < last_local_time - 5min` → `CLOCK_ROLLBACK_DETECTED`,禁用 Pro
  (正常 NTP 微调不会误判)

### 8.7 Grace Period(客户端本地维护,默认 72h)

服务端只负责状态判定;宽限由客户端保存 `last_successful_verify` 实现:
- Server 不可达(SSE 断开 / Verify 网络失败)→ 宽限期内继续 Pro
- **宽限必须有上限**(默认 72h,`DM_LICENSE_GRACE_PERIOD` 可覆盖):到期 → `grace_expired` → 禁用 Pro
- **恢复 Server 的唯一正常机制:SSE Reconnect Success = Server Recovery Signal → 立即 V3 Verify**
- `revoked/blocked` 不能进入宽限:服务端明确判定时立即禁用;已判定状态不被 Server 不可达覆盖
- 宽限评估发生在每次 Verify 失败 / SSE 断开(重连尝试本身即评估点,无需周期验证)

### 8.8 安全事件(管理端)

`POST /api/v1/admin/security-events?page=&page_size=&type=` 可查:
`invalid_signature` / `invalid_token` / `rate_limit_exceeded` / `replay_detected` /
`tampered_timestamp` / `device_limit_exceeded` / `client_version_blocked` 等。
安全事件只记录非敏感标识(license_id/activation_id/device_id/ip),
**绝不记录完整 License Key / Activation Token / 私钥**。

---

## 9. 管理 API(V3 新增)

| 端点 | 说明 |
|---|---|
| `POST/GET /api/v1/admin/customers` | 客户管理(CUS-*) |
| `POST/GET /api/v1/admin/subscriptions` | 订阅管理(SUB-*;支持 active/expired/cancelled/suspended) |
| `POST /api/v1/admin/subscriptions/:id/status` | 更新订阅状态 |
| `GET /api/v1/admin/security-events` | 安全事件列表(分页,可按 type 过滤) |
| `GET/PUT /api/v1/admin/settings` | 服务器配置:`minimum_client_version` / `blocked_versions`(JSON 数组) |
| `POST /api/v1/admin/licenses` | 签发(可带 `customer_id` / `subscription_id`) |
| `POST /api/v1/admin/licenses/:id/revoke` | 吊销(`reason` + `revoked_by` 记录) |

## 10. 数据库(V3 migration 003)

```
customers ──< subscriptions ──< licenses ──< license_revisions
                                        └──< activations ──< activation_tokens(只存 SHA-256)
security_events / security_nonces / server_settings / signing_keys / audit_logs / admins
```

- `activations`:新增 `activation_id`(ACT-*)/ `device_fingerprint` / `platform` /
  `architecture` / `expires_at` / `revoked_at`
- `activation_tokens`:凭据只存 `SHA-256(token)` hash,明文绝不落库
- 存量 `activations.activation_code`(明文)在 003 migration 中迁移为 token hash 后清空
- `licenses`:新增 `customer_ref` / `subscription_id` / `revoked_by`(可空,存量兼容)
