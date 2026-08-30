#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Docker_Manager_License 本地 UI 预览用 mock Admin API(Python 标准库,零依赖)。

用法:
  1. python web/scripts/mock-admin-api.py     # 监听 127.0.0.1:3000
  2. cd web && npm run dev                    # vite 代理 /api -> 127.0.0.1:3000
  3. 浏览器打开 http://localhost:5173 (admin / admin123)

数据字段与真实后端 internal/api/* 完全一致(仅供 UI 验收,非真实数据)。
"""
import json
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

NOW = int(time.time())
DAY = 86400


def ts(days_from_now):
    return NOW + days_from_now * DAY


def ulid(prefix):
    return prefix + "-" + uuid.uuid4().hex[:26].upper()


def base_license(license_id, customer, plan, status, created_days_ago, expires_days_from_now,
                 features=None, max_devices=1, active_devices=0, key_id="dD7pzvzY", revoked_reason="", notes=""):
    return {
        "id": license_id.lower(), "license_id": license_id, "key_id": key_id,
        "product": "docker-manager-go", "plan": plan,
        "features": features or ["compose", "container_create", "appstore"],
        "customer": customer, "issued_at": ts(-created_days_ago), "expires_at": ts(expires_days_from_now),
        "max_devices": max_devices, "active_devices": active_devices, "status": status,
        "revoked_reason": revoked_reason, "notes": notes,
        "created_at": __import__("datetime").datetime.fromtimestamp(ts(-created_days_ago)).isoformat() + "Z",
    }


LICENSES = [
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V2", "Zhao Studio", "pro", "active", 120, 245, max_devices=3, active_devices=2),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V3", "Acme Corp", "pro", "active", 90, 12, max_devices=5, active_devices=5),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V4", "TechFlow", "pro", "active", 60, 365, max_devices=1, active_devices=1),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V5", "云启科技", "pro", "active", 30, 88, max_devices=2, active_devices=0),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V6", "Zhao Studio", "pro", "expired", 400, -10, max_devices=1, active_devices=0),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V7", "NorthWind", "pro", "revoked", 200, 100, max_devices=2, active_devices=1,
                 revoked_reason="Fraud"),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V8", "BetaLab", "pro", "suspended", 45, 300, max_devices=1, active_devices=1),
    base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1V9", "Acme Corp", "pro", "active", 15, 350, max_devices=10, active_devices=3,
                 notes="企业年度授权"),
]

CUSTOMERS = [
    {"id": "1", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VW", "name": "Zhao Studio", "email": "zhao@example.com",
     "status": "active", "created_at": "2026-01-15T08:00:00Z", "updated_at": "2026-01-15T08:00:00Z"},
    {"id": "2", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VX", "name": "Acme Corp", "email": "ops@acme.example",
     "status": "active", "created_at": "2026-02-01T08:00:00Z", "updated_at": "2026-02-01T08:00:00Z"},
    {"id": "3", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VY", "name": "云启科技", "email": "",
     "status": "active", "created_at": "2026-03-10T08:00:00Z", "updated_at": "2026-03-10T08:00:00Z"},
]

SUBSCRIPTIONS = [
    {"id": "1", "subscription_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1VZ", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VW",
     "plan": "pro", "status": "active", "starts_at": ts(-120), "expires_at": ts(245), "auto_renew": True,
     "created_at": "2026-05-01T00:00:00Z", "updated_at": "2026-05-01T00:00:00Z"},
    {"id": "2", "subscription_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1W0", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VX",
     "plan": "pro", "status": "active", "starts_at": ts(-90), "expires_at": ts(12), "auto_renew": False,
     "created_at": "2026-06-01T00:00:00Z", "updated_at": "2026-06-01T00:00:00Z"},
    {"id": "3", "subscription_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1W1", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VY",
     "plan": "pro", "status": "suspended", "starts_at": ts(-45), "expires_at": ts(300), "auto_renew": True,
     "created_at": "2026-07-10T00:00:00Z", "updated_at": "2026-07-10T00:00:00Z"},
    {"id": "4", "subscription_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1W2", "customer_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VW",
     "plan": "pro", "status": "cancelled", "starts_at": ts(-400), "expires_at": ts(-10), "auto_renew": False,
     "created_at": "2025-08-01T00:00:00Z", "updated_at": "2025-08-01T00:00:00Z"},
]

SECURITY_EVENTS = [
    {"id": 1, "event_type": "replay_detected", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4",
     "activation_id": "ACT-01J2K3M4N5P6Q7R8S9T0U1W3", "device_id": "51703-2635@docker-manager",
     "ip": "1.2.3.4", "user_agent": "docker-manager-go/1.3.2", "details": "nonce reused within 1h window",
     "created_at": "2026-08-30T09:12:00Z"},
    {"id": 2, "event_type": "invalid_signature", "license_id": "", "activation_id": "", "device_id": "",
     "ip": "5.6.7.8", "user_agent": "", "details": "key parse failed", "created_at": "2026-08-30T08:45:00Z"},
    {"id": 3, "event_type": "rate_limit_exceeded", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V5",
     "activation_id": "", "device_id": "", "ip": "9.9.9.9", "user_agent": "", "details": "activate: 20 req / 15min",
     "created_at": "2026-08-30T07:30:00Z"},
    {"id": 4, "event_type": "client_version_blocked", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V3",
     "activation_id": "ACT-01J2K3M4N5P6Q7R8S9T0U1W4", "device_id": "a1b2c3@docker-manager",
     "ip": "10.0.0.2", "user_agent": "docker-manager-go/1.2.5", "details": "version 1.2.5 is blocked",
     "created_at": "2026-08-29T22:10:00Z"},
    {"id": 5, "event_type": "tampered_timestamp", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4",
     "activation_id": "", "device_id": "51703-2635@docker-manager", "ip": "1.2.3.4",
     "user_agent": "docker-manager-go/1.3.2", "details": "clock offset > 5min",
     "created_at": "2026-08-29T20:00:00Z"},
    {"id": 6, "event_type": "device_limit_exceeded", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V3",
     "activation_id": "", "device_id": "new-device-99", "ip": "10.0.0.9", "user_agent": "docker-manager-go/1.3.2",
     "details": "max_devices=5 reached", "created_at": "2026-08-29T18:22:00Z"},
    {"id": 7, "event_type": "invalid_token", "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V8",
     "activation_id": "", "device_id": "unknown", "ip": "77.88.99.1", "user_agent": "",
     "details": "activation_token hash mismatch", "created_at": "2026-08-29T15:05:00Z"},
]

AUDIT_LOGS = [
    {"id": 10, "admin": "admin", "action": "license.create", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V9", "ip": "127.0.0.1", "metadata": '{"plan":"pro","max_devices":10}',
     "created_at": "2026-08-30T10:02:00Z"},
    {"id": 9, "admin": "admin", "action": "license.revoke", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V7", "ip": "127.0.0.1", "metadata": '{"reason":"Fraud"}',
     "created_at": "2026-08-30T09:40:00Z"},
    {"id": 8, "admin": "admin", "action": "license.extend", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V2", "ip": "127.0.0.1", "metadata": '{"days":365,"reason":"renewal"}',
     "created_at": "2026-08-30T09:15:00Z"},
    {"id": 7, "admin": "admin", "action": "customer.create", "resource_type": "customer",
     "resource_id": "CUS-01J2K3M4N5P6Q7R8S9T0U1VY", "ip": "127.0.0.1", "metadata": '{"name":"云启科技"}',
     "created_at": "2026-08-30T08:55:00Z"},
    {"id": 6, "admin": "admin", "action": "subscription.create", "resource_type": "subscription",
     "resource_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1W1", "ip": "127.0.0.1", "metadata": '{"plan":"pro"}',
     "created_at": "2026-08-30T08:50:00Z"},
    {"id": 5, "admin": "admin", "action": "license.activate", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4", "ip": "1.2.3.4", "metadata": '{"device":"51703-2635@docker-manager"}',
     "created_at": "2026-08-30T08:30:00Z"},
    {"id": 4, "admin": "admin", "action": "settings.update", "resource_type": "setting",
     "resource_id": "minimum_client_version", "ip": "127.0.0.1", "metadata": '{"value":"1.4.0"}',
     "created_at": "2026-08-30T07:00:00Z"},
    {"id": 3, "admin": "admin", "action": "license.create", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V5", "ip": "127.0.0.1", "metadata": '{"plan":"pro"}',
     "created_at": "2026-08-29T22:00:00Z"},
    {"id": 2, "admin": "admin", "action": "subscription.status", "resource_type": "subscription",
     "resource_id": "SUB-01J2K3M4N5P6Q7R8S9T0U1W1", "ip": "127.0.0.1", "metadata": '{"status":"suspended"}',
     "created_at": "2026-08-29T20:00:00Z"},
    {"id": 1, "admin": "admin", "action": "license.create", "resource_type": "license",
     "resource_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V2", "ip": "127.0.0.1", "metadata": '{"plan":"pro"}',
     "created_at": "2026-08-29T10:00:00Z"},
]

SIGNING_KEYS = [
    {"key_id": "dD7pzvzY", "algorithm": "Ed25519", "public_key": "dD7pzvzY8dLcVfQdU6nQe2vKqZgY4sLpT9wQxN0mRjBkHc",
     "status": "active", "created_at": "2026-08-01T00:00:00Z", "retired_at": None},
    {"key_id": "NVF2xwYq", "algorithm": "Ed25519", "public_key": "NVF2xwYq8dLcVfQdU6nQe2vKqZgY4sLpT9wQxN0mRjBkHc",
     "status": "retired", "created_at": "2026-07-01T00:00:00Z", "retired_at": "2026-08-01T00:00:00Z"},
]

SETTINGS = {"minimum_client_version": "1.4.0", "blocked_versions": '["1.2.5"]'}

ACTIVATIONS = [
    {"id": 1, "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4", "activation_code": "ACT-01J2K3M4N5P6Q7R8S9T0U1W3",
     "device_id": "51703-2635@docker-manager", "device_name": "VPS-Prod", "product_version": "1.3.2",
     "status": "active", "activated_at": "2026-08-15T10:00:00Z", "last_seen_at": "2026-08-30T09:00:00Z",
     "deactivated_at": None, "ip": "1.2.3.4"},
    {"id": 2, "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4", "activation_code": "ACT-01J2K3M4N5P6Q7R8S9T0U1W5",
     "device_id": "8f3a1c@docker-manager", "device_name": "NAS-Backup", "product_version": "1.3.0",
     "status": "active", "activated_at": "2026-08-20T14:00:00Z", "last_seen_at": "2026-08-29T22:00:00Z",
     "deactivated_at": None, "ip": "10.0.0.5"},
    {"id": 3, "license_id": "DMG-01J2K3M4N5P6Q7R8S9T0U1V4", "activation_code": "ACT-01J2K3M4N5P6Q7R8S9T0U1W6",
     "device_id": "old-laptop", "device_name": "MacBook-Pro", "product_version": "1.2.9",
     "status": "deactivated", "activated_at": "2026-08-10T09:00:00Z", "last_seen_at": "2026-08-18T09:00:00Z",
     "deactivated_at": "2026-08-20T09:00:00Z", "ip": "192.168.1.8"},
]

REVISIONS = [
    {"id": 2, "revision": 2, "payload": "eyJ2ZXJzaW9uIjoyfQ", "signature": "sig...", "reason": "renewal",
     "created_by": "admin", "created_at": "2026-08-30T09:15:00Z"},
    {"id": 1, "revision": 1, "payload": "eyJ2ZXJzaW9uIjoyfQ", "signature": "sig...", "reason": "issue",
     "created_by": "admin", "created_at": "2026-08-15T10:00:00Z"},
]

LICENSE_BY_ID = {l["license_id"]: l for l in LICENSES}


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, obj=None):
        body = json.dumps(obj, ensure_ascii=False).encode() if obj is not None else b""
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)

    def _auth(self):
        auth = self.headers.get("Authorization", "")
        if not auth.startswith("Bearer "):
            self._send(401, {"error": {"code": "UNAUTHORIZED", "message": "未登录或登录已过期"}})
            return False
        return True

    def _page(self, items):
        q = parse_qs(urlparse(self.path).query)
        page = int(q.get("page", ["1"])[0])
        page_size = int(q.get("page_size", ["20"])[0])
        start = (page - 1) * page_size
        return {"items": items[start:start + page_size], "total": len(items), "page": page, "page_size": page_size}

    def do_GET(self):
        p = self.path.split("?")[0]
        if p == "/api/v1/admin/login" or p == "/healthz":
            return self._send(200, {"status": "ok"})
        if not self._auth():
            return

        if p == "/api/v1/admin/me":
            return self._send(200, {"username": "admin"})
        if p == "/api/v1/admin/stats":
            by_status = {"active": 0, "expired": 0, "revoked": 0, "suspended": 0}
            for l in LICENSES:
                by_status[l["status"]] = by_status.get(l["status"], 0) + 1
            return self._send(200, {"total": len(LICENSES), "by_status": by_status})
        if p == "/api/v1/admin/licenses":
            status = parse_qs(urlparse(self.path).query).get("status", [""])[0]
            items = [l for l in LICENSES if not status or l["status"] == status]
            return self._send(200, self._page(items))
        if p.startswith("/api/v1/admin/licenses/") and p.endswith("/revisions"):
            return self._send(200, {"items": REVISIONS})
        if p.startswith("/api/v1/admin/licenses/") and p.endswith("/activations"):
            return self._send(200, {"items": ACTIVATIONS})
        if p.startswith("/api/v1/admin/licenses/"):
            lid = p.rsplit("/", 1)[-1]
            l = LICENSE_BY_ID.get(lid)
            if not l:
                return self._send(404, {"error": {"code": "LICENSE_NOT_FOUND", "message": "license not found"}})
            return self._send(200, {"license": l})
        if p == "/api/v1/admin/customers":
            return self._send(200, self._page(CUSTOMERS))
        if p == "/api/v1/admin/subscriptions":
            return self._send(200, self._page(SUBSCRIPTIONS))
        if p == "/api/v1/admin/security-events":
            t = parse_qs(urlparse(self.path).query).get("type", [""])[0]
            items = [e for e in SECURITY_EVENTS if not t or e["event_type"] == t]
            return self._send(200, self._page(items))
        if p == "/api/v1/admin/audit-logs":
            return self._send(200, self._page(AUDIT_LOGS))
        if p == "/api/v1/admin/settings":
            return self._send(200, SETTINGS)
        if p == "/api/v1/admin/signing-keys":
            return self._send(200, {"items": SIGNING_KEYS})
        self._send(404, {"error": {"code": "NOT_FOUND", "message": "not found: " + p}})

    def do_POST(self):
        p = self.path.split("?")[0]
        if p == "/api/v1/admin/login":
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            return self._send(200, {"token": "mock-token-" + uuid.uuid4().hex, "username": body.get("username", "admin")})
        if not self._auth():
            return
        if p == "/api/v1/admin/licenses":
            return self._send(201, {
                "license": base_license("DMG-01J2K3M4N5P6Q7R8S9T0U1ZZ", "新建客户", "pro", "active", 0, 365),
                "key": "eyJ2ZXJzaW9uIjoyLCJrZXlfaWQiOiJkRDdwenZ6WSIsImxpY2Vuc2VfaWQiOiJETUctMDFKMkszTTRONUw2UTdSOFM5VDBVMVpaIn0.mock-signature-" + uuid.uuid4().hex,
                "payload": "eyJ2ZXJzaW9uIjoyfQ",
            })
        if "/revoke" in p:
            return self._send(200, {"ok": True})
        if "/extend" in p:
            return self._send(200, {"ok": True})
        if "/reset-devices" in p:
            return self._send(200, {"ok": True})
        if "/deactivate" in p:
            return self._send(200, {"ok": True})
        if p == "/api/v1/admin/customers":
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            c = {"id": str(len(CUSTOMERS) + 1), "customer_id": "CUS-" + uuid.uuid4().hex[:26].upper(),
                 "name": body.get("name", ""), "email": body.get("email", ""), "status": "active",
                 "created_at": "2026-08-30T10:00:00Z", "updated_at": "2026-08-30T10:00:00Z"}
            CUSTOMERS.insert(0, c)
            return self._send(201, {"customer": c})
        if p == "/api/v1/admin/subscriptions":
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            s = {"id": str(len(SUBSCRIPTIONS) + 1), "subscription_id": "SUB-" + uuid.uuid4().hex[:26].upper(),
                 "customer_id": body.get("customer_id", ""), "plan": body.get("plan", "pro"),
                 "status": "active", "starts_at": body.get("starts_at", NOW), "expires_at": body.get("expires_at", NOW + 365 * DAY),
                 "auto_renew": body.get("auto_renew", False), "created_at": "2026-08-30T10:00:00Z",
                 "updated_at": "2026-08-30T10:00:00Z"}
            SUBSCRIPTIONS.insert(0, s)
            return self._send(201, {"subscription": s})
        if "/status" in p:
            return self._send(200, {"ok": True})
        if p == "/api/v1/admin/setup-totp":
            return self._send(200, {"secret": "JBSWY3DPEHPK3PXP"})
        if p == "/api/v1/admin/confirm-totp" or p == "/api/v1/admin/disable-totp" or p == "/api/v1/admin/change-password" or p == "/api/v1/admin/logout":
            return self._send(200, {"ok": True})
        self._send(404, {"error": {"code": "NOT_FOUND", "message": "not found: " + p}})

    def do_PUT(self):
        p = self.path.split("?")[0]
        if not self._auth():
            return
        if p == "/api/v1/admin/settings":
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            SETTINGS[body.get("key", "")] = body.get("value", "")
            return self._send(200, {"ok": True})
        self._send(404, {"error": {"code": "NOT_FOUND", "message": "not found: " + p}})


if __name__ == "__main__":
    print("mock admin API on http://127.0.0.1:3000 (login: admin / admin123)")
    ThreadingHTTPServer(("127.0.0.1", 3000), H).serve_forever()
