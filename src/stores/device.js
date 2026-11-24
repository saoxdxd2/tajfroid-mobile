import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useDeviceStore = defineStore('device', () => {
  const deviceToken = ref(localStorage.getItem('deviceToken') || '')
  const deviceId = ref(localStorage.getItem('deviceId') || '')
  const relayUrl = ref(localStorage.getItem('relayUrl') || '')
  const desktopPublicKey = ref(localStorage.getItem('desktopPublicKey') || '')

  async function pair(pairingCode) {
    const res = await fetch(`${relayUrl.value}/api/pair`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ pairingCode, deviceName: 'phone' }),
    })
    if (!res.ok) throw new Error('pair failed')
    const data = await res.json()
    deviceToken.value = data.deviceToken
    deviceId.value = data.deviceId
    localStorage.setItem('deviceToken', data.deviceToken)
    localStorage.setItem('deviceId', data.deviceId)
  }

  function setRelayUrl(url) { relayUrl.value = url; localStorage.setItem('relayUrl', url) }
  function setDesktopPublicKey(key) { desktopPublicKey.value = key; localStorage.setItem('desktopPublicKey', key) }

  // Called when the relay rejects a stored token. Keep the relay address and
  // public key so re-pairing can reuse them, but clear the stale identity.
  function clearSession() {
    deviceToken.value = ''
    deviceId.value = ''
    localStorage.removeItem('deviceToken')
    localStorage.removeItem('deviceId')
  }

  return { deviceToken, deviceId, relayUrl, desktopPublicKey, pair, setRelayUrl, setDesktopPublicKey, clearSession }
})