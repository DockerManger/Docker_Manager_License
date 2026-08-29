-- 003_license_v3.sql — License V3 架构升级
--
-- 新增:
--   customers / subscriptions            License 与客户、订阅分离(Skill §7-9)
--   activation_tokens                   激活凭据(只存 SHA-256,绝不明文;§11)
--   security_events                     安全事件体系(§20)
--   security_nonces                     重放防护(§16)
--   server_settings                     minimum_client_version / blocked_versions(§21)
--   activations 扩展                     activation_id 展示 ID / device_fingerprint / platform /
--                                        architecture / expires_at / revoked_at(§10,§18)
--   licenses 扩展                        customer_ref / subscription_id / revoked_by(§8,§19)
--
-- 兼容性:
--   - 存量 activation_code(明文)迁移为 token hash:旧客户端升级后本地旧凭据仍可验证,
--     迁移完成后清空明文列(数据库不再保存明文 Token)
--   - licenses 新列为可空,存量 License 不受影响

-- ============ 1. Customer / Subscription(License 与客户身份分离) ============

CREATE TABLE customers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id TEXT NOT NULL UNIQUE,             -- CUS-<ULID>,展示用
    name        TEXT NOT NULL,
    email       TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'active',   -- active / disabled
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id TEXT NOT NULL UNIQUE,         -- SUB-<ULID>,展示用
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    plan            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active', -- active / expired / cancelled / suspended
    starts_at       BIGINT NOT NULL,
    expires_at      BIGINT NOT NULL,
    auto_renew      BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscriptions_customer ON subscriptions (customer_id);
CREATE INDEX idx_subscriptions_status ON subscriptions (status);

-- licenses 关联 customer / subscription(可空,兼容存量 License)
ALTER TABLE licenses ADD COLUMN customer_ref UUID REFERENCES customers(id);
ALTER TABLE licenses ADD COLUMN subscription_id UUID REFERENCES subscriptions(id);
ALTER TABLE licenses ADD COLUMN revoked_by TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_licenses_customer_ref ON licenses (customer_ref);

-- ============ 2. Activation 升级:展示 ID / 指纹 / 平台 / 过期 / 吊销 ============

ALTER TABLE activations
    ADD COLUMN activation_id      TEXT NOT NULL DEFAULT '',  -- ACT-<ULID>,展示用
    ADD COLUMN device_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN platform           TEXT NOT NULL DEFAULT '',
    ADD COLUMN architecture       TEXT NOT NULL DEFAULT '',
    ADD COLUMN expires_at         TIMESTAMPTZ,               -- 激活有效期(随 License 过期)
    ADD COLUMN revoked_at         TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_activations_activation_id ON activations (activation_id) WHERE activation_id <> '';

-- 存量激活记录回填展示 ID
UPDATE activations SET activation_id = 'ACT-' || replace(gen_random_uuid()::text, '-', '')
WHERE activation_id = '';

-- ============ 3. Activation Token(只存 SHA-256,数据库绝不明文) ============

CREATE TABLE activation_tokens (
    id            BIGSERIAL PRIMARY KEY,
    activation_id BIGINT NOT NULL REFERENCES activations(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,           -- SHA-256(hex)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL DEFAULT now() + interval '365 days',
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX idx_activation_tokens_activation ON activation_tokens (activation_id);

-- 存量 activation_code(明文)迁移为 token hash:
-- 旧客户端本地保存的旧凭据(activation_id=旧 code)升级后仍可完成 verify/deactivate
INSERT INTO activation_tokens (activation_id, token_hash, created_at)
SELECT id, encode(sha256(activation_code::bytea), 'hex'), now()
FROM activations WHERE activation_code <> '';

-- 迁移完成后清空明文列:数据库不再保存明文 Token(新流程完全不读写该列)
UPDATE activations SET activation_code = '';

-- ============ 4. Security Events(不记录敏感信息:key/token/私钥) ============

CREATE TABLE security_events (
    id            BIGSERIAL PRIMARY KEY,
    event_type    TEXT NOT NULL,     -- invalid_signature / invalid_token / replay_detected / ...
    license_id    TEXT NOT NULL DEFAULT '',
    activation_id TEXT NOT NULL DEFAULT '',
    device_id     TEXT NOT NULL DEFAULT '',
    ip            TEXT NOT NULL DEFAULT '',
    user_agent    TEXT NOT NULL DEFAULT '',
    details       TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_security_events_created_at ON security_events (created_at);
CREATE INDEX idx_security_events_type ON security_events (event_type);

-- ============ 5. Replay Protection Nonce(SHA-256 存储,定期清理) ============

CREATE TABLE security_nonces (
    nonce_hash TEXT PRIMARY KEY,    -- SHA-256(nonce)(hex)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_security_nonces_created_at ON security_nonces (created_at);

-- ============ 6. 服务器配置(客户端版本控制等) ============

CREATE TABLE server_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO server_settings (key, value) VALUES
    ('minimum_client_version', ''),
    ('blocked_versions', '[]');
