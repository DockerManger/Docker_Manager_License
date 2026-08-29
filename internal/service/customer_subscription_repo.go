package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
)

// ---------- CustomerRepository(V3:客户身份独立) ----------

// CustomerRepo 客户存取。
type CustomerRepo struct{ pool *pgxpool.Pool }

// NewCustomerRepo 构造。
func NewCustomerRepo(pool *pgxpool.Pool) *CustomerRepo { return &CustomerRepo{pool: pool} }

// Create 创建客户。
func (r *CustomerRepo) Create(ctx context.Context, c *model.Customer) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO customers (customer_id, name, email, status)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at, updated_at`,
		c.CustomerID, c.Name, c.Email, c.Status,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

// GetByCustomerID 按展示 ID 查询。
func (r *CustomerRepo) GetByCustomerID(ctx context.Context, customerID string) (*model.Customer, error) {
	var c model.Customer
	err := r.pool.QueryRow(ctx, `
		SELECT id, customer_id, name, email, status, created_at, updated_at
		FROM customers WHERE customer_id = $1`, customerID).
		Scan(&c.ID, &c.CustomerID, &c.Name, &c.Email, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List 分页列表。
func (r *CustomerRepo) List(ctx context.Context, offset, limit int) ([]*model.Customer, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM customers`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, customer_id, name, email, status, created_at, updated_at
		FROM customers ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.Customer, 0)
	for rows.Next() {
		var c model.Customer
		if err := rows.Scan(&c.ID, &c.CustomerID, &c.Name, &c.Email, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	return out, total, rows.Err()
}

// ---------- SubscriptionRepository(V3:订阅独立) ----------

// SubscriptionRepo 订阅存取。
type SubscriptionRepo struct{ pool *pgxpool.Pool }

// NewSubscriptionRepo 构造。
func NewSubscriptionRepo(pool *pgxpool.Pool) *SubscriptionRepo { return &SubscriptionRepo{pool: pool} }

// Create 创建订阅。
func (r *SubscriptionRepo) Create(ctx context.Context, s *model.Subscription) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (subscription_id, customer_id, plan, status, starts_at, expires_at, auto_renew)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, created_at, updated_at`,
		s.SubscriptionID, s.CustomerID, s.Plan, s.Status, s.StartsAt, s.ExpiresAt, s.AutoRenew,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

// GetBySubscriptionID 按展示 ID 查询(join 客户带出 CUS-*)。
func (r *SubscriptionRepo) GetBySubscriptionID(ctx context.Context, subscriptionID string) (*model.Subscription, error) {
	var s model.Subscription
	err := r.pool.QueryRow(ctx, `
		SELECT s.id, s.subscription_id, c.customer_id, s.plan, s.status, s.starts_at, s.expires_at,
		       s.auto_renew, s.created_at, s.updated_at
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		WHERE s.subscription_id = $1`, subscriptionID).
		Scan(&s.ID, &s.SubscriptionID, &s.CustomerID, &s.Plan, &s.Status, &s.StartsAt,
			&s.ExpiresAt, &s.AutoRenew, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByCustomer 某客户的全部订阅。
func (r *SubscriptionRepo) ListByCustomer(ctx context.Context, customerDBID string) ([]*model.Subscription, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.subscription_id, c.customer_id, s.plan, s.status, s.starts_at, s.expires_at,
		       s.auto_renew, s.created_at, s.updated_at
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		WHERE s.customer_id = $1 ORDER BY s.created_at DESC`, customerDBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*model.Subscription, 0)
	for rows.Next() {
		var s model.Subscription
		if err := rows.Scan(&s.ID, &s.SubscriptionID, &s.CustomerID, &s.Plan, &s.Status, &s.StartsAt,
			&s.ExpiresAt, &s.AutoRenew, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// List 分页列表。
func (r *SubscriptionRepo) List(ctx context.Context, offset, limit int) ([]*model.Subscription, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.subscription_id, c.customer_id, s.plan, s.status, s.starts_at, s.expires_at,
		       s.auto_renew, s.created_at, s.updated_at
		FROM subscriptions s
		JOIN customers c ON c.id = s.customer_id
		ORDER BY s.created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*model.Subscription, 0)
	for rows.Next() {
		var s model.Subscription
		if err := rows.Scan(&s.ID, &s.SubscriptionID, &s.CustomerID, &s.Plan, &s.Status, &s.StartsAt,
			&s.ExpiresAt, &s.AutoRenew, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, &s)
	}
	return out, total, rows.Err()
}

// UpdateStatus 更新订阅状态。
func (r *SubscriptionRepo) UpdateStatus(ctx context.Context, id, status string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE subscriptions SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDBIDBySubscriptionID 按展示 ID 取数据库 UUID(供 licenses 外键)。
func (r *SubscriptionRepo) GetDBIDBySubscriptionID(ctx context.Context, subscriptionID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `SELECT id FROM subscriptions WHERE subscription_id = $1`, subscriptionID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// touchUpdatedAt 供内部使用(保留)。
var _ = time.Now
