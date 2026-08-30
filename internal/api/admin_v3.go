package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/license"
	"github.com/DockerManger/Docker_Manager_License/internal/model"
	"github.com/DockerManger/Docker_Manager_License/internal/service"
)

// ---------- Customers(V3:客户管理) ----------

type customerRequest struct {
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
	Status string `json:"status,omitempty"`
}

// adminCreateCustomer POST /api/v1/admin/customers
func adminCreateCustomer(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req customerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "name is required")
			return
		}
		status := req.Status
		if status == "" {
			status = "active"
		}
		cust := &model.Customer{
			CustomerID: license.NewCustomerID(),
			Name:       name,
			Email:      strings.TrimSpace(req.Email),
			Status:     status,
		}
		if err := d.CustomerRepo.Create(c.Request.Context(), cust); err != nil {
			handleError(c, err)
			return
		}
		_ = d.AuditRepo.Log(c.Request.Context(), &model.AuditLog{
			Admin: c.GetString(ctxAdminKey), Action: "customer.create",
			ResourceType: "customer", ResourceID: cust.CustomerID, IP: clientIP(c),
		})
		c.JSON(http.StatusCreated, gin.H{"customer": cust})
	}
}

// adminListCustomers GET /api/v1/admin/customers?page=&page_size=
func adminListCustomers(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pagination(c)
		items, total, err := d.CustomerRepo.List(c.Request.Context(), (page-1)*pageSize, pageSize)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	}
}

// ---------- Subscriptions(V3:订阅管理) ----------

type subscriptionRequest struct {
	CustomerID string `json:"customer_id"`
	Plan       string `json:"plan"`
	Status     string `json:"status,omitempty"`
	StartsAt   int64  `json:"starts_at"`
	ExpiresAt  int64  `json:"expires_at"`
	AutoRenew  bool   `json:"auto_renew,omitempty"`
}

// adminCreateSubscription POST /api/v1/admin/subscriptions
func adminCreateSubscription(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		cust, err := d.CustomerRepo.GetByCustomerID(c.Request.Context(), strings.TrimSpace(req.CustomerID))
		if err != nil {
			if err == service.ErrNotFound {
				abort(c, http.StatusBadRequest, "BAD_REQUEST", "customer not found")
				return
			}
			handleError(c, err)
			return
		}
		if req.ExpiresAt <= 0 {
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "expires_at is required")
			return
		}
		status := req.Status
		if status == "" {
			status = "active"
		}
		sub := &model.Subscription{
			SubscriptionID: license.NewSubscriptionID(),
			CustomerID:     cust.CustomerID,
			Plan:           req.Plan,
			Status:         status,
			StartsAt:       req.StartsAt,
			ExpiresAt:      req.ExpiresAt,
			AutoRenew:      req.AutoRenew,
		}
		if err := d.SubscriptionRepo.Create(c.Request.Context(), sub, cust.ID); err != nil {
			handleError(c, err)
			return
		}
		_ = d.AuditRepo.Log(c.Request.Context(), &model.AuditLog{
			Admin: c.GetString(ctxAdminKey), Action: "subscription.create",
			ResourceType: "subscription", ResourceID: sub.SubscriptionID, IP: clientIP(c),
		})
		c.JSON(http.StatusCreated, gin.H{"subscription": sub})
	}
}

// adminListSubscriptions GET /api/v1/admin/subscriptions?page=&page_size=
func adminListSubscriptions(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pagination(c)
		items, total, err := d.SubscriptionRepo.List(c.Request.Context(), (page-1)*pageSize, pageSize)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	}
}

// adminUpdateSubscriptionStatus POST /api/v1/admin/subscriptions/:id/status
type subStatusRequest struct {
	Status string `json:"status"`
}

func adminUpdateSubscriptionStatus(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		sub, err := d.SubscriptionRepo.GetBySubscriptionID(c.Request.Context(), c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		if err := d.SubscriptionRepo.UpdateStatus(c.Request.Context(), sub.ID, req.Status); err != nil {
			handleError(c, err)
			return
		}
		_ = d.AuditRepo.Log(c.Request.Context(), &model.AuditLog{
			Admin: c.GetString(ctxAdminKey), Action: "subscription.status",
			ResourceType: "subscription", ResourceID: sub.SubscriptionID, IP: clientIP(c),
			Metadata: `{"status":"` + req.Status + `"}`,
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// ---------- Security Events(V3:安全事件) ----------

// adminSecurityEvents GET /api/v1/admin/security-events?page=&page_size=&type=
func adminSecurityEvents(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, pageSize := pagination(c)
		eventType := strings.TrimSpace(c.Query("type"))
		items, total, err := d.Security.List(c.Request.Context(), (page-1)*pageSize, pageSize, eventType)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
	}
}

// ---------- Server Settings(V3:minimum_client_version / blocked_versions) ----------

// adminSettings GET /api/v1/admin/settings
func adminSettings(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		all, err := d.Settings.All(c.Request.Context())
		if err != nil {
			handleError(c, err)
			return
		}
		out := map[string]any{}
		for _, s := range all {
			out[s.Key] = s.Value
		}
		c.JSON(http.StatusOK, out)
	}
}

// adminUpdateSettings PUT /api/v1/admin/settings
// 允许的 key:minimum_client_version(版本字符串)、blocked_versions(JSON 数组)。
type settingsUpdateRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func adminUpdateSettings(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingsUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			handleError(c, err)
			return
		}
		key := strings.TrimSpace(req.Key)
		switch key {
		case "minimum_client_version":
			// 任意非空版本字符串
		case "blocked_versions":
			// 必须是合法 JSON 字符串数组
			var list []string
			if req.Value != "" {
				if err := json.Unmarshal([]byte(req.Value), &list); err != nil {
					abort(c, http.StatusBadRequest, "BAD_REQUEST", "blocked_versions must be a JSON string array")
					return
				}
			}
		default:
			abort(c, http.StatusBadRequest, "BAD_REQUEST", "unknown setting key")
			return
		}
		if err := d.Settings.Set(c.Request.Context(), key, strings.TrimSpace(req.Value)); err != nil {
			handleError(c, err)
			return
		}
		// V3:版本策略变化 → 全局广播事件(所有 SSE 订阅者收到后自动 Verify 获取新策略)
		_ = d.LicenseSvc.PublishGlobal(c.Request.Context(), service.EvtVersionPolicyChanged, map[string]any{
			"key": key, "value": strings.TrimSpace(req.Value),
		})
		_ = d.AuditRepo.Log(c.Request.Context(), &model.AuditLog{
			Admin: c.GetString(ctxAdminKey), Action: "settings.update",
			ResourceType: "setting", ResourceID: key, IP: clientIP(c),
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
