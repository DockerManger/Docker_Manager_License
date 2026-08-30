-- 004_license_v3_events.sql — License V3 Event-Driven 主动同步(SSE)
--
-- 新增:
--   license_events                    持久化 Event Store(事件溯源/Replay/Last-Event-ID)
--   activations.state_version         状态版本(防旧事件覆盖新状态 / 乱序保护)
--
-- 设计(与 Docker_Manager_Go 客户端契约一致):
--   - id(BIGSERIAL) = sequence_id,单调递增,用于 Replay / Ordering / Gap Detection
--   - event_id = 'evt_' || id(Last-Event-ID 即 evt_<sequence>)
--   - 事件按 activation_id(展示 ID)路由;activation_id = '' 表示全局广播事件
--     (如 version_policy.changed),所有订阅者都会收到且参与 Replay
--   - state_version 随每次状态变更 +1:解绑/吊销/重新激活/许可证状态变化
--   - 禁止保存敏感数据:不存 License Key / 私钥 / activation_token 明文
--   - 事务一致性:状态变更与事件 INSERT 在同一事务内完成,提交后再 Publish

CREATE TABLE license_events (
    id            BIGSERIAL PRIMARY KEY,        -- sequence_id(单调递增);event_id = 'evt_' || id
    event_type    TEXT NOT NULL,                -- license.changed / activation.revoked / ...
    license_id    TEXT NOT NULL DEFAULT '',     -- 展示 ID(DMG-*)
    activation_id TEXT NOT NULL DEFAULT '',     -- 展示 ID(ACT-*);'' = 全局广播
    device_id     TEXT NOT NULL DEFAULT '',
    state_version BIGINT NOT NULL DEFAULT 0,
    payload       JSONB NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_license_events_activation ON license_events (activation_id, id);
CREATE INDEX idx_license_events_license ON license_events (license_id, id);
CREATE INDEX idx_license_events_type ON license_events (event_type);
CREATE INDEX idx_license_events_created_at ON license_events (created_at);

-- 激活状态版本:每次状态变更 +1(初始 1)
ALTER TABLE activations ADD COLUMN state_version BIGINT NOT NULL DEFAULT 1;
