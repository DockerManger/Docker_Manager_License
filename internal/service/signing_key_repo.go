package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MinimaxFlora/Docker_Manager_License/internal/model"
)

// ---------- SigningKeyRepository ----------
//
// 签名密钥注册表:key rotation 的基础。旧公钥永不删除(旧 License 仍需验证),
// 只通过 status 标记 retired/revoked。当前密钥以私钥文件为准:
// 启动时 Ensure 注册,key_id 相同但公钥不同(本地 keygen 重建)时覆盖更新。

// SigningKeyRepo 签名密钥注册表存取。
type SigningKeyRepo struct{ pool *pgxpool.Pool }

// NewSigningKeyRepo 构造。
func NewSigningKeyRepo(pool *pgxpool.Pool) *SigningKeyRepo { return &SigningKeyRepo{pool: pool} }

// Ensure 注册/更新当前签发密钥(key_id 为主键,公钥以私钥文件为准)。
func (r *SigningKeyRepo) Ensure(ctx context.Context, keyID, algorithm, pubB64 string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO signing_keys (key_id, algorithm, public_key, status)
		VALUES ($1,$2,$3,'active')
		ON CONFLICT (key_id) DO UPDATE SET algorithm = EXCLUDED.algorithm, public_key = EXCLUDED.public_key`,
		keyID, algorithm, pubB64)
	return err
}

// GetPublicKey 按 key_id 取公钥(base64url)。查不到返回 ErrNotFound。
func (r *SigningKeyRepo) GetPublicKey(ctx context.Context, keyID string) (string, error) {
	var pub string
	err := r.pool.QueryRow(ctx,
		`SELECT public_key FROM signing_keys WHERE key_id = $1`, keyID).Scan(&pub)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return pub, nil
}

// List 全部密钥记录(创建时间倒序)。
func (r *SigningKeyRepo) List(ctx context.Context) ([]*model.SigningKey, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT key_id, algorithm, public_key, status, created_at, retired_at
		FROM signing_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.SigningKey, 0)
	for rows.Next() {
		var k model.SigningKey
		if err := rows.Scan(&k.KeyID, &k.Algorithm, &k.PublicKey, &k.Status, &k.CreatedAt, &k.RetiredAt); err != nil {
			return nil, err
		}
		out = append(out, &k)
	}
	return out, rows.Err()
}
