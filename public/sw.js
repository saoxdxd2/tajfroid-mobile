// Offline app shell only. API data, auth, QR responses and ciphertext are
// never intercepted or stored here.
const CACHE_NAME = 'tajfroid-shell-v1'

const SHELL_PATHS = [
  '/',
  '/index.html',
  '/manifest.json',
  '/manifest.webmanifest',
  '/favicon.svg',
  '/icons.svg',
  '/pwa-512x512.png',
]

const ASSET_URL_RE = /\/assets\/[A-Za-z0-9._/-]+/g
const SODIUM_CHUNK_RE = /libsodium-wrappers-[A-Za-z0-9._-]+\.js/g
const STATIC_ASSET_RE = /^\/(?:assets\/[A-Za-z0-9._/-]+|favicon\.svg|icons\.svg|manifest\.json|manifest\.webmanifest|pwa-512x512\.png)$/

// Cache each URL separately: a missing icon or asset must not abort install.
async function cacheUrl(cache, path, seen) {
  if (seen.has(path)) return null
  seen.add(path)
  try {
    const response = await fetch(path, { cache: 'reload' })
    if (!response || !response.ok || response.type === 'opaque') return null
    await cache.put(path, response.clone())
    return response
  } catch {
    return null
  }
}

async function precacheShell() {
  const cache = await caches.open(CACHE_NAME)
  const seen = new Set()

  for (const path of SHELL_PATHS) await cacheUrl(cache, path, seen)

  const htmlResponse = (await cache.match('/index.html')) || (await cache.match('/'))
  const html = htmlResponse ? await htmlResponse.text() : ''
  const sources = [html]

  // Hashed entry assets are discovered from the built index.html.
  for (const match of html.matchAll(ASSET_URL_RE)) {
    const response = await cacheUrl(cache, match[0], seen)
    if (response && match[0].endsWith('.js')) sources.push(await response.text())
  }

  // The lazy libsodium chunk is referenced from the entry JS, not the HTML.
  for (const source of sources) {
    for (const match of source.matchAll(SODIUM_CHUNK_RE)) {
      await cacheUrl(cache, `/assets/${match[0]}`, seen)
    }
  }
}

self.addEventListener('install', event => {
  event.waitUntil(precacheShell().catch(() => {}).then(() => self.skipWaiting()))
})

self.addEventListener('activate', event => {
  event.waitUntil((async () => {
    const keys = await caches.keys()
    await Promise.all(keys
      .filter(key => key.startsWith('tajfroid-shell-') && key !== CACHE_NAME)
      .map(key => caches.delete(key)))
    await self.clients.claim()
  })())
})

self.addEventListener('fetch', event => {
  const request = event.request
  if (request.method !== 'GET') return

  const url = new URL(request.url)
  if (url.origin !== self.location.origin) return
  if (url.pathname.startsWith('/api/')) return

  // Navigations: always try the network, fall back to the cached shell.
  // Route responses are never cached.
  if (request.mode === 'navigate') {
    event.respondWith((async () => {
      try {
        return await fetch(request)
      } catch {
        const cache = await caches.open(CACHE_NAME)
        return (await cache.match('/index.html')) || (await cache.match('/')) || Response.error()
      }
    })())
    return
  }

  // Static shell assets: cache-first, revalidated in the background.
  if (STATIC_ASSET_RE.test(url.pathname)) {
    const cachePromise = caches.open(CACHE_NAME)
    const revalidate = cachePromise.then(cache =>
      fetch(request).then(response => {
        if (response && response.ok && response.type === 'basic') {
          return cache.put(url.pathname, response.clone()).then(() => response, () => response)
        }
        return response
      })
    ).catch(() => null)
    event.waitUntil(revalidate)

    event.respondWith((async () => {
      const cache = await cachePromise
      const cached = await cache.match(url.pathname)
      if (cached) return cached
      return (await revalidate) || Response.error()
    })())
  }
})
