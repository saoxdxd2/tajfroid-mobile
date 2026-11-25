const HLC_STORAGE_KEY = 'taj_hlc_last'

export function getDeviceId() {
  if (typeof localStorage === 'undefined') return 'phone-local'
  let id = localStorage.getItem('deviceId') || ''
  if (!id) {
    id = localStorage.getItem('taj_client_device_id') || ''
    if (!id) {
      id = `phone-${(typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID().slice(0, 8) : Math.random().toString(36).slice(2, 10))}`
      localStorage.setItem('taj_client_device_id', id)
    }
  }
  return id
}

function parseHlc(raw) {
  if (!raw || typeof raw !== 'string') return { wall: 0, logical: 0, device: '' }
  const parts = raw.split(':')
  if (parts.length < 3) return { wall: 0, logical: 0, device: '' }
  return {
    wall: Number(parts[0]) || 0,
    logical: Number(parts[1]) || 0,
    device: parts.slice(2).join(':'),
  }
}

export function nextHlc(remoteRaw = '') {
  const prevRaw = typeof localStorage !== 'undefined' ? (localStorage.getItem(HLC_STORAGE_KEY) || '') : ''
  const prev = parseHlc(prevRaw)
  const remote = parseHlc(remoteRaw)
  const now = Date.now()
  const maxWall = Math.max(now, prev.wall, remote.wall)
  let logical = 0
  if (maxWall === prev.wall && maxWall === remote.wall) {
    logical = Math.max(prev.logical, remote.logical) + 1
  } else if (maxWall === prev.wall) {
    logical = prev.logical + 1
  } else if (maxWall === remote.wall) {
    logical = remote.logical + 1
  }
  const hlc = `${String(maxWall).padStart(16, '0')}:${String(logical).padStart(6, '0')}:${getDeviceId()}`
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(HLC_STORAGE_KEY, hlc)
  }
  return hlc
}

export function buildFieldOp({ entity = 'catalog', entityId, field, value, baseHlc = '' }) {
  return {
    opId: crypto.randomUUID(),
    deviceId: getDeviceId(),
    entity,
    entityId,
    opKind: 'field_set',
    field,
    value,
    hlc: nextHlc(baseHlc),
    baseHlc,
  }
}

export function buildStockDeltaOp({ entityId, deltaInt, baseHlc = '' }) {
  return {
    opId: crypto.randomUUID(),
    deviceId: getDeviceId(),
    entity: 'catalog',
    entityId,
    opKind: 'stock_delta',
    deltaInt: Number(deltaInt) || 0,
    hlc: nextHlc(baseHlc),
    baseHlc,
  }
}
