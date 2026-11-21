// Tiny API client: bearer session from /api/auth (initData → session token, telegram-core).
let token = ''
export const setToken = (t) => (token = t)
export const getToken = () => token

export class ApiError extends Error {
  constructor(status, code, message) { super(message); this.status = status; this.code = code }
}

export async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data?.error?.code, data?.error?.message || res.statusText)
  return data
}
