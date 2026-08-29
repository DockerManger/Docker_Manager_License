-- 001_init.sql — 初始 Schema
-- 所有 Schema 变更必须新增版本化 migration 文件,禁止运行时自动猜结构 ALTER。

-- 管理端账号
CREATE TABLE admins (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    token_version INTEGER NOT NULL DEFAULT 0,
    totp_secret   TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 许可证主记录
CREATE TABLE licenses (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    license_id     TEXT NOT NULL UNIQUE,        -- DMG-<ULID>,展示用
    key_id         TEXT NOT NULL,               -- 签发密钥标识(轮换支持)
    product        TEXT NOT NULL DEFAULT 'docker-manager-go',
    plan           TEXT NOT NULL,
    features       TEXT[] NOT NULL DEFAULT '{}',
    customer       TEXT NOT NULL DEFAULT '',
    issued_at      BIGINT NOT NULL,
    expires_at     BIGINT NOT NULL,
    max_devices    INTEGER NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'active',  -- active/expired/revoked/suspended
    revoked_at     BIGINT,
    revoked_reason TEXT NOT NULL DEFAULT '',
    notes          TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_licenses_status ON licenses (status);
CREATE INDEX idx_licenses_customer ON licenses (customer);
CREATE INDEX idx_licenses_expires_at ON licenses (expires_at);

-- License 历史修订(每次签发/延期/变更新增一行,不覆盖)
CREATE TABLE license_revisions (
    id         BIGSERIAL PRIMARY KEY,
    license_id UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    revision   INTEGER NOT NULL,
    payload    TEXT NOT NULL,          -- 规范 JSON(签名前内容)
    signature  TEXT NOT NULL,          -- base64url 签名
    key        TEXT NOT NULL,          -- 完整 Key 字符串
    reason     TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (license_id, revision)
);

-- 设备激活记录(第一版预留;离线绑定由消费端本地完成)
CREATE TABLE activations (
    id             BIGSERIAL PRIMARY KEY,
    license_id     UUID NOT NULL REFERENCES licenses(id) ON DELETE CASCADE,
    device_id      TEXT NOT NULL,
    device_name    TEXT NOT NULL DEFAULT '',
    activated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ,
    ip             TEXT NOT NULL DEFAULT '',
    metadata       TEXT NOT NULL DEFAULT '',
    UNIQUE (license_id, device_id)
);

-- 审计日志
CREATE TABLE audit_logs (
    id            BIGSERIAL PRIMARY KEY,
    admin         TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id   TEXT NOT NULL DEFAULT '',
    ip            TEXT NOT NULL DEFAULT '',
    metadata      TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at);
CREATE INDEX idx_audit_logs_action ON audit_logs (action);
