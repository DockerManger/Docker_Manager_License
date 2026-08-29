package auth

import (
	"sync"
	"time"
)

// ---------- 登录限流 ----------
//
// IP 级滑动窗口:15 分钟内累计 10 次失败 → 锁定 15 分钟(避免暴力破解)。
// 内存实现,单实例足够;多实例部署时需换 Redis(第一版不需要)。

type failEntry struct {
	failures int
	windowAt time.Time
	lockedAt time.Time
}

// LoginLimiter IP 登录失败限流器。
type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]*failEntry

	window   time.Duration // 失败计数窗口
	maxFails int           // 窗口内最大失败次数
	lockDur  time.Duration // 锁定时长
}

// NewLoginLimiter 构造限流器。
func NewLoginLimiter(window time.Duration, maxFails int, lockDur time.Duration) *LoginLimiter {
	return &LoginLimiter{
		entries:  make(map[string]*failEntry),
		window:   window,
		maxFails: maxFails,
		lockDur:  lockDur,
	}
}

// Allow 返回该 IP 当前是否允许尝试登录。
func (l *LoginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[ip]
	if !ok {
		return true
	}
	now := time.Now()
	if !e.lockedAt.IsZero() {
		if now.Sub(e.lockedAt) >= l.lockDur {
			// 锁定到期,重置
			delete(l.entries, ip)
			return true
		}
		return false
	}
	if now.Sub(e.windowAt) >= l.window {
		delete(l.entries, ip)
		return true
	}
	return e.failures < l.maxFails
}

// RecordFailure 记录一次失败。
func (l *LoginLimiter) RecordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	e, ok := l.entries[ip]
	if !ok {
		e = &failEntry{windowAt: now}
		l.entries[ip] = e
	}
	if now.Sub(e.windowAt) >= l.window {
		e.windowAt = now
		e.failures = 0
	}
	e.failures++
	if e.failures >= l.maxFails {
		e.lockedAt = now
	}
}

// RecordSuccess 登录成功后清除计数。
func (l *LoginLimiter) RecordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}
