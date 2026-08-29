# Docker_Manager_Go 集成文档（License V2 契约）

> 本文档是 **Docker_Manager_License（签发端）** 与 **Docker_Manager_Go（消费端）** 之间唯一的正式契约。
> 任何一方的修改必须同步另一方，否则授权失效。本仓库 `internal/integration/consumer_test.go`
> 用独立实现模拟了消费端验证逻辑，改造 Docker_Manager_Go 时以此为准。

---

## 1. 数据流

```
Docker_Manager_License                    Docker_Manager_Go (开源)
        │                                        │
   Ed25519 私钥                                 Ed25519 公钥(内嵌)
        │                                        │
   签发 Key 字符串                              Parse → Verify → Expiry → Feature
        │                                        │
        └──────── license.json ─────────────────┘
                 (离线,无需联网)
```

**安全模型**：
- 攻击者拿到 Docker_Manager_Go 全部源码 → 只能看到公钥，**不能生成合法 License**
- 攻击者拿到 License 文件 → 不能修改有效期/features/max_devices（签名失效）
- 攻击者复制 License → 离线模式由 max_devices/本地绑定规则约束

---

## 1.5 对外 API Base URL(唯一规范)

生产环境固定唯一 Base URL(规格书 §22,禁止多套前缀):

```
https://manager.kejizero.xyz/license-api
```

客户端请求 = **Base URL + 内部 API 路径**:

| 操作 | 完整 URL(对外) | 内部路径(license-server) |
|---|---|---|
| 激活 | `https://manager.kejizero.xyz/license-api/api/v1/public/activate` | `/api/v1/public/activate` |
| 验证 | `https://manager.kejizero.xyz/license-api/api/v1/public/verify` | `/api/v1/public/verify` |
| 解绑 | `https://manager.kejizero.xyz/license-api/api/v1/public/deactivate` | `/api/v1/public/deactivate` |
| 健康检查 | `https://manager.kejizero.xyz/license-api/health` | `/health`(别名 `/healthz` 兼容) |
| 管理后台 | `https://manager.kejizero.xyz/`(SPA) | 前端 + `/api/v1/admin/*` |

**前缀剥离**:Caddy 反代配置 `uri strip_prefix /license-api` 后转发到
license-server 容器(`license-server:3000`),license-server 内部**没有**
`/license-api` 前缀——所有内部路由以 `/api/v1/...` 开头。

开发/私有部署可用环境变量 `DM_LICENSE_SERVER_URL` 覆盖客户端地址;
License Server 侧 `SERVER_ADDR` 控制监听(默认 `:3000`)。



---

## 2. Key 字符串格式

```
<base64url(规范JSON payload)>.<base64url(Ed25519签名 64 字节)>
```

示例（结构）：
```
eyJ2ZXJzaW9uIjoyLCJrZXlfaWQiOiIyMDI2LTAxIiwibGljZW5zZV9pZCI6IkRNRy0wMUowMDAw...}.<88位签名>
```

**V1 已彻底删除**(规格书 §31):V1 HMAC 验证路径、`licenseSecret`、`LicenseSign`
已从 Docker_Manager_Go 移除,不再保留 V1/V2 双协议。消费端只接受 V2(Ed25519)。
存量 V1 Key 无法激活,需重新签发 V2 License。

---

## 3. Payload 规范（V2）

字段顺序即规范 JSON 序列化顺序（Go struct 字段序），**签名覆盖完整 payload 字节**。

```json
{
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

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | int | 必须为 `2`；遇到更高版本必须拒绝（`UNSUPPORTED_LICENSE_VERSION`），不得静默接受 |
| `key_id` | string | 签发密钥标识（轮换/泄露处理用），如 `2026-01` |
| `license_id` | string | 全局唯一展示 ID（客服查询凭据），前缀 `DMG-` |
| `product` | string | 固定 `docker-manager-go`，不符直接拒绝 |
| `plan` | string | `pro`（第一版；`free` 不需要 License） |
| `features` | string[] | Feature Registry 子集（见下） |
| `customer` | string | 客户名 |
| `issued_at` / `expires_at` | int64 | Unix 秒；`expires_at < now` → expired |
| `max_devices` | int | 允许绑定设备数（第一版消费端单机绑定语义，>=1） |

## 4. Feature Registry（两端必须逐字一致）

| Feature | Docker_Manager_Go 门控点 |
|---|---|
| `compose` | `internal/service/compose.go` 部署入口（现 `license.required` 403） |
| `container_create` | `internal/service/container.go` 创建入口 |
| `appstore` | `internal/service/appstore.go` 安装入口 |

> ⚠️ 严禁两端各自起名（如 `advanced-compose` vs `compose_advanced`）。新增 Feature 必须先改本 Registry + 本仓库 `internal/license/format.go`，再同步消费端。

---

## 5. Docker_Manager_Go 改造清单

基于对当前源码（`internal/service/license.go` 为 HMAC v1）的调研，正式接入 V2 需要修改以下文件：

### 5.1 `internal/service/license.go`（核心）

```go
// 1. 新增 V2 公钥常量(由 License Server 提供,key_id → 公钥映射)
var licensePublicKeys = map[string]ed25519.PublicKey{
    "2026-01": mustDecodePubKey(`-----BEGIN PUBLIC KEY-----
...（部署时由签发端 keygen 输出）
-----END PUBLIC KEY-----`),
}

// 2. LicenseVerifyKey 入口按 §2 规则分派:
//    第二段 32 hex → V1 HMAC(现有逻辑保留)
//    否则 → V2:license.VerifyKey(key, pub) (见下方参考实现)

// 3. 功能门控:新增按 feature 判断(兼容旧 key 视为全功能)
//    LicenseFeatureActive(st, "compose")
//    旧 V1 key(type==pro 且有效) → 全部 features 视为开启(迁移兼容)
//    新 V2 key → 按 payload.features 精确判断

// 4. 删除/禁用 LicenseGenerateKey 的生产使用 —— HMAC secret 已公开,
//    继续用它签发等于免费授权。仅保留测试用途(或整体移除)。
```

### 5.2 `internal/api/license.go`

- `licenseDemoKey`：V2 时代改为返回由**测试专用密钥对**签发的 demo key（或直接下线该接口）。
  现有 SEC-001（仅 `DisplayVersion()=="unknown"` 的开发构建可用）保护机制保留。

### 5.3 `internal/service/compose.go` / `container.go` / `appstore.go`

- `license: func() bool { return LicenseActive(st) }` → 改为按 feature：
  ```go
  license: func() bool { return service.LicenseFeatureActive(st, "compose") }
  ```

### 5.4 `web/src/views/SettingsView.vue` + `web/src/locales/*`

- 许可证表格增加列：License ID、features（badge）、max_devices
- 激活流程不变（粘贴 key / 上传 .lic 文件均兼容 V2 字符串）
- 新增 i18n keys（14 语言全量同步）

### 5.5 `internal/service/license.go` 消费端参考实现（V2）

```go
// V2VerifyKey 消费端 Ed25519 验证(与签发端契约一致,改动需同步两端)
func V2VerifyKey(key string, pub ed25519.PublicKey) (map[string]any, bool) {
    parts := strings.SplitN(strings.TrimSpace(key), ".", 2)
    if len(parts) != 2 { return nil, false }
    raw, err := base64.RawURLEncoding.DecodeString(parts[0])
    if err != nil { return nil, false }
    sig, err := base64.RawURLEncoding.DecodeString(parts[1])
    if err != nil || len(sig) != ed25519.SignatureSize { return nil, false }
    var p struct {
        Version   int      `json:"version"`
        KeyID     string   `json:"key_id"`
        LicenseID string   `json:"license_id"`
        Product   string   `json:"product"`
        Plan      string   `json:"plan"`
        Features  []string `json:"features"`
        Customer  string   `json:"customer"`
        IssuedAt  int64    `json:"issued_at"`
        ExpiresAt int64    `json:"expires_at"`
        MaxDevices int     `json:"max_devices"`
    }
    if json.Unmarshal(raw, &p) != nil { return nil, false }
    if p.Version != 2 { return nil, false }                 // UNSUPPORTED_LICENSE_VERSION
    if p.Product != "docker-manager-go" { return nil, false }
    if !ed25519.Verify(pub, raw, sig) { return nil, false } // 篡改即失败
    out := map[string]any{
        "version": p.Version, "key_id": p.KeyID, "license_id": p.LicenseID,
        "plan": p.Plan, "features": p.Features, "customer": p.Customer,
        "issued_at": p.IssuedAt, "expires_at": p.ExpiresAt, "max_devices": p.MaxDevices,
        "status": "active",
    }
    if p.ExpiresAt > 0 && p.ExpiresAt < time.Now().Unix() { out["status"] = "expired" }
    return out, true
}
```

---

## 6. 部署步骤（接入正式环境）

1. 部署 License Server（见 README），首次启动记录输出的 **PUBLIC KEY**
2. 把公钥写入 `internal/service/license.go` 的 `licensePublicKeys` 映射（key_id 与 Server 的 `LICENSE_KEY_ID` 一致）
3. 按 §5 改造 Docker_Manager_Go 并发布
4. 用 License Server 签发一个测试 License → 在面板粘贴激活 → 验证 Pro 生效
5. 回归验证：旧 V1 Key 仍能激活（双轨兼容期），随后择机迁移存量用户

## 7. 兼容性策略

- **V1 已彻底删除**(2026-08 重构,规格书 §31):客户端只验证 V2 Ed25519,
  License Server 只签发 V2;不存在 V1/V2 双轨。
- License payload 带 `version`,未来 v3 由消费端明确拒绝而不是静默接受

## 8. 在线授权闭环 API(客户端接入)

客户端(消费端)导入 License 并通过本地 Ed25519 验签后,进入在线闭环:

### 8.1 激活

```
POST /api/v1/public/activate
{ "key": "<完整 License Key>", "device_id": "<机器唯一ID>",
  "device_name": "可选", "product_version": "可选" }
```

成功(200):
```json
{ "status": "active", "activation_id": "<64位hex>", "license_id": "DMG-...",
  "expires_at": 1810000000, "features": ["compose"], "max_devices": 3,
  "next_verify_after": 86400 }
```

失败(统一错误体):`INVALID_SIGNATURE` / `LICENSE_NOT_FOUND` / `LICENSE_REVOKED` /
`LICENSE_EXPIRED` / `DEVICE_LIMIT_REACHED`。

语义:
- 同一设备重复激活 → 幂等(200,返回原 activation_id)
- 解绑过的设备重新激活 → 恢复 active,发新 activation_id,不占新额度
- 活跃设备数 >= max_devices → `DEVICE_LIMIT_REACHED`(服务端事务+行锁,并发激活不突破)

### 8.2 定期验证(每 24h,即 next_verify_after)

```
POST /api/v1/public/verify
{ "key": "...", "activation_id": "...", "device_id": "...", "product_version": "可选" }
```

返回:
```json
{ "status": "valid", "valid": true, "license_id": "DMG-...", "plan": "pro",
  "customer": "Zhao", "expires_at": 1810000000, "features": ["compose"],
  "next_verify_after": 86400 }
```

| status | 客户端动作 |
|---|---|
| `valid` | 继续 Pro,记录 `last_successful_verify`,24h 后再验 |
| `revoked` | 立即禁用 Pro,提示"License revoked" |
| `expired` | 立即禁用 Pro(本地 expires_at 也应同时判断) |
| `invalid` | 设备未激活/凭据不匹配,禁用 Pro |

兼容:仅传 `key`(不带 device_id)→ 返回 License 在线状态,不校验设备(旧调用兼容)。

### 8.3 解绑

```
POST /api/v1/public/deactivate
{ "key": "...", "activation_id": "...", "device_id": "..." }
```

- 必须携带激活时返回的 `activation_id`,防止 Device A 解绑 Device B(不匹配 → `ACTIVATION_NOT_FOUND`)
- 吊销/过期的 License 也允许解绑(客户端清理)

### 8.4 限流

`activate`/`deactivate`:15min 20 次/IP;`verify`:15min 120 次/IP。超限 `RATE_LIMITED`。

### 8.5 Grace Period(客户端本地维护)

服务端只负责状态判定;7 天宽限由客户端保存 `last_successful_verify` 实现:
验证失败(网络/服务不可达)→ 宽限期内继续 Pro;超过宽限期仍未验证成功 → 禁用 Pro。
**验证必须带超时(建议 10s),且不得阻塞 Docker_Manager_Go 主流程(独立后台任务)。**
