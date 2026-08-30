-- 005_license_lifecycle.sql — License 生命周期重构(解绑 ≠ 吊销)
--
-- 目标(与 Docker_Manager_Go 客户端契约一致):
--   1. 一个 Docker_Manager_Go 实例(device_id)同时只能绑定一个 License:
--      同一 device_id 至多存在一条 status='active' 的激活记录(partial unique index)。
--      这是"一个 DMG 只能绑定一个 License"的数据库级最终一致性保证,
--      即使并发激活(同设备不同 License)也只能成功一条。
--   2. 解绑(用户/管理员)只是把激活置为 deactivated —— License 本身保持 active,
--      允许重新激活;只有吊销才把 License 置为 revoked。
--   3. 删除仅允许 REVOKED(由 service 层校验,本 migration 不处理)。
--
-- 兼容性:不删除任何存量数据,不 DROP TABLE。存量冲突按"保留最早激活,
-- 其余解绑"处理,License 主记录全部保留。

-- ============ 1. 清理存量冲突:同一 device_id 多条 active 激活 ============
-- 保留 id 最小(最早)的一条,其余置 deactivated(不是 revoked,许可证不受影响)。
UPDATE activations a
SET status = 'deactivated', deactivated_at = now()
WHERE a.status = 'active'
  AND a.id NOT IN (
      SELECT MIN(id) FROM activations
      WHERE status = 'active' AND device_id <> ''
      GROUP BY device_id
  );

-- ============ 2. 唯一约束:一个设备同时最多一个 active 激活 ============
-- 数据库级兜底:任何路径(含绕过 service 层的并发激活)都无法让同一
-- device_id 同时存在两条 active 激活记录。
CREATE UNIQUE INDEX uq_activations_active_device
    ON activations (device_id)
    WHERE status = 'active' AND device_id <> '';
