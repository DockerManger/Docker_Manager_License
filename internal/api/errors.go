// Package api Gin HTTP 层:路由、中间件、请求/响应。
// 业务逻辑一律在 service 层,API 层只做参数解析与错误映射。
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/DockerManger/Docker_Manager_License/internal/model"
	"github.com/DockerManger/Docker_Manager_License/internal/service"
)

// ---------- 统一错误结构 ----------
//
// 响应格式: {"error": {"code": "LICENSE_NOT_FOUND", "message": "..."}}
// 禁止把 SQL error / 文件路径 / 私钥路径 / 堆栈 直接返回客户端。

// ErrorBody 统一错误体。
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail 错误详情。
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ApiError 带 HTTP 状态的业务错误。
type ApiError struct {
	Status  int
	Code    string
	Message string
}

// Error 实现 error 接口。
func (e *ApiError) Error() string { return e.Code + ": " + e.Message }

// NewApiError 构造。
func NewApiError(status int, code, msg string) *ApiError {
	return &ApiError{Status: status, Code: code, Message: msg}
}

// errorCodes 常见错误码。
var (
	ErrUnauthorized = NewApiError(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrForbidden    = NewApiError(http.StatusForbidden, "FORBIDDEN", "permission denied")
	ErrRateLimited  = NewApiError(http.StatusTooManyRequests, "RATE_LIMITED", "too many attempts, try later")
)

// abort 写统一错误响应。
func abort(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, ErrorBody{Error: ErrorDetail{Code: code, Message: msg}})
}

// handleError 把任意 error 映射为统一响应。
func handleError(c *gin.Context, err error) {
	var ae *ApiError
	if errors.As(err, &ae) {
		abort(c, ae.Status, ae.Code, ae.Message)
		return
	}
	switch {
	case errors.Is(err, service.ErrLicenseNotFound):
		abort(c, http.StatusNotFound, "LICENSE_NOT_FOUND", "license not found")
	case errors.Is(err, service.ErrLicenseRevoked):
		abort(c, http.StatusForbidden, "LICENSE_REVOKED", "license has been revoked")
	case errors.Is(err, service.ErrLicenseExpired):
		abort(c, http.StatusBadRequest, "LICENSE_EXPIRED", "license has expired")
	case errors.Is(err, service.ErrDeviceLimit):
		abort(c, http.StatusConflict, "DEVICE_LIMIT_REACHED", "device limit reached for this license")
	case errors.Is(err, service.ErrDeviceBound):
		abort(c, http.StatusConflict, "DEVICE_BOUND", "device is already bound to another license, unbind it first")
	case errors.Is(err, service.ErrActivationMismatch):
		abort(c, http.StatusNotFound, "ACTIVATION_NOT_FOUND", "activation not found or does not match device")
	case errors.Is(err, service.ErrInvalidSignature):
		abort(c, http.StatusBadRequest, "INVALID_SIGNATURE", "invalid license key signature")
	case errors.Is(err, service.ErrReplayDetected):
		abort(c, http.StatusBadRequest, "REPLAY_DETECTED", "replay detected: timestamp outside window or nonce reused")
	case errors.Is(err, service.ErrTokenInvalid):
		abort(c, http.StatusBadRequest, "TOKEN_INVALID", "invalid activation token")
	case errors.Is(err, service.ErrTokenExpired):
		abort(c, http.StatusUnauthorized, "TOKEN_EXPIRED", "activation token expired")
	case errors.Is(err, service.ErrClientVersionBlocked):
		abort(c, http.StatusForbidden, "CLIENT_VERSION_BLOCKED", "client version blocked, please update")
	case errors.Is(err, service.ErrActivationMissing):
		abort(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, service.ErrNotFound):
		abort(c, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, service.ErrConflict):
		abort(c, http.StatusConflict, "CONFLICT", err.Error())
	default:
		abort(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
}

// secEvent 构造安全事件(API 层辅助;不记录敏感信息)。
func secEvent(eventType, licenseID, activationID, deviceID, ip, details string) *model.SecurityEvent {
	return &model.SecurityEvent{
		EventType:    eventType,
		LicenseID:    licenseID,
		ActivationID: activationID,
		DeviceID:     deviceID,
		IP:           ip,
		Details:      details,
	}
}
