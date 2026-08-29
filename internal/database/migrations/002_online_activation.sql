-- 002_online_activation.sql — 在线授权闭环:激活记录增强 + 签名密钥注册表
-- 上一版 activations 仅为"预留表"(离线验证由消费端本地绑定);
-- 本版接入在线激活:激活凭据、设备状态、产品版本、签名密钥注册表。

-- 签名密钥注册表(key rotation 基础:旧公钥永不删除,只改 status)
CREATE TABLE signing_keys (
    key_id     TEXT PRIMARY KEY,
    algorithm  TEXT NOT NULL DEFAULT 'ed25519',
    public_key TEXT NOT NULL,                       -- base64url(32 字节公钥)
    status     TEXT NOT NULL DEFAULT 'active',      -- active / retired / revoked
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retired_at TIMESTAMPTZ
);

-- activations 表增强:在线激活字段
ALTER TABLE activations
    ADD COLUMN activation_code TEXT NOT NULL DEFAULT '',  -- 激活凭据(客户端 deactivate/verify 携带,防跨设备操作)
    ADD COLUMN product_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active';      -- active / deactivated

CREATE UNIQUE INDEX idx_activations_code ON activations (activation_code) WHERE activation_code <> '';
CREATE INDEX idx_activations_license_status ON activations (license_id, status);
