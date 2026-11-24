// Registers the offline app shell service worker in production builds only.
export function registerAppShell() {
  if (!import.meta.env?.PROD) return
  if (typeof window === 'undefined' || !('serviceWorker' in navigator)) return

  const register = () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {})
  }

  if (document.readyState === 'complete') register()
  else window.addEventListener('load', register, { once: true })
}

// Removes every service worker registration and all app-shell caches.
export async function unregisterAppShell() {
  if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
    try {
      const registrations = await navigator.serviceWorker.getRegistrations()
      await Promise.all(registrations.map(registration =>
        registration.unregister().catch(() => {})))
    } catch {
      // Nothing to unregister.
    }
  }

  if (typeof caches !== 'undefined') {
    try {
      const keys = await caches.keys()
      await Promise.all(keys
        .filter(key => key.startsWith('tajfroid-shell-'))
        .map(key => caches.delete(key).catch(() => {})))
    } catch {
      // Nothing to delete.
    }
  }
}
