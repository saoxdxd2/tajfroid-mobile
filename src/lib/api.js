import router from '../router'

export function getRelayUrl() {
  return localStorage.getItem('relayUrl') || ''
}

export function getDeviceToken() {
  return localStorage.getItem('deviceToken') || ''
}

export function authHeaders() {
  const token = getDeviceToken()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export function jsonHeaders() {
  return { 'Content-Type': 'application/json', ...authHeaders() }
}

export async function readError(res, fallback = 'Request failed') {
  const text = await res.text().catch(() => '')
  if (!text) return fallback
  try {
    const data = JSON.parse(text)
    if (typeof data === 'string') return data
    if (data?.error) return data.error
    if (data?.message) return data.message
  } catch {
    // plain text response
  }
  return text
}

function shouldRedirectToPair(path) {
  if (path.startsWith('/api/pair')) return false
  if (path.startsWith('/api/agent/bootstrap')) return false
  return true
}

export async function apiFetch(path, options = {}) {
  const base = getRelayUrl()
  if (!base) throw new Error('Relay URL is not configured')
  const url = path.startsWith('http') ? path : `${base}${path}`
  const headers = { ...(options.headers || {}) }
  // Attach auth unless caller explicitly set Authorization or opted out.
  if (!headers.Authorization && options.auth !== false) {
    const token = getDeviceToken()
    if (token) headers.Authorization = `Bearer ${token}`
  }
  const res = await fetch(url, { ...options, headers })
  if (res.status === 401 && shouldRedirectToPair(path)) {
    // Token missing/expired/revoked. Keep relay URL but force re-pair.
    localStorage.removeItem('deviceToken')
    localStorage.removeItem('deviceId')
    if (router.currentRoute.value.path !== '/pair') {
      router.push({ path: '/pair', query: { reason: 'expired' } }).catch(() => {})
    }
  }
  return res
}

// A UNIQUE conflict on the client-supplied id/sku is an idempotent replay, not
// a business conflict. Other 409s (duplicate reference, stale version) stay
// visible with their original message.
export async function readConflictResponse(res) {
  if (res.status !== 409) return null
  const text = await res.text().catch(() => '')
  let error = text
  try {
    const d = JSON.parse(text)
    error = d?.error || d?.message || text
  } catch { /* plain text */ }
  return {
    text, error,
    duplicate: /UNIQUE[^\n]*\.(id|sku)\b|duplicate[^\n]*\b(id|sku)\b/i.test(text),
  }
}

export async function apiJson(path, options = {}) {
  const res = await apiFetch(path, options)
  if (!res.ok) throw new Error(await readError(res, `Request failed (${res.status})`))
  if (res.status === 204) return null
  return res.json()
}
