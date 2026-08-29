// API 封装:统一处理 JSON / 错误结构 / token。
const TOKEN_KEY = 'dml_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(path, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  let data: any = null
  const text = await res.text()
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }
  if (!res.ok) {
    const code = data?.error?.code || 'ERROR'
    const message = data?.error?.message || res.statusText
    if (res.status === 401) clearToken()
    throw new ApiError(res.status, code, message)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
}

// ---------- 类型 ----------

export interface License {
  id: string
  license_id: string
  key_id: string
  product: string
  plan: string
  features: string[]
  customer: string
  issued_at: number
  expires_at: number
  max_devices: number
  active_devices: number
  status: string
  revoked_reason: string
  notes: string
  created_at: string
}

export interface Activation {
  id: number
  license_id: string
  activation_code: string
  device_id: string
  device_name: string
  product_version: string
  status: string
  activated_at: string
  last_seen_at: string
  deactivated_at: string | null
  ip: string
}

export interface SigningKey {
  key_id: string
  algorithm: string
  public_key: string
  status: string
  created_at: string
}

export interface LicenseRevision {
  id: number
  revision: number
  payload: string
  signature: string
  reason: string
  created_by: string
  created_at: string
}

export interface AuditLog {
  id: number
  admin: string
  action: string
  resource_type: string
  resource_id: string
  ip: string
  metadata: string
  created_at: string
}

export interface Page<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export const FEATURES = ['compose', 'container_create', 'appstore']
export const PLANS = ['pro']

export const FEATURE_LABELS: Record<string, string> = {
  compose: 'Compose 编排',
  container_create: '容器创建',
  appstore: '应用商店',
}
