// User-facing copy for the phone app. Technical errors stay out of the UI.
export const COPY = {
  brand: 'TAJ FROID',

  connectToComputer: 'Connect to computer',
  connectedToLaptop: 'Connected to your laptop',
  notConnected: 'Connect this phone to your computer to sync your work.',

  offlineBanner: 'You are offline. Your changes are saved here and will sync when you reconnect.',
  offlineError: 'You seem to be offline. Check your connection and try again.',
  sessionExpired: 'Your connection expired. Please connect to your computer again.',
  conflict: 'Another device changed this record. Review the difference before retrying.',
  duplicate: 'That reference is already in use. Try a different one.',
  serverProblem: 'The server is having trouble right now. Please try again shortly.',
  requestFailed: 'Something went wrong. Please try again.',

  accountConnected: 'Connected to your laptop. Your changes sync automatically when you are online.',
  accountNotConnected: 'This phone is not connected yet. Pair it with your computer to start syncing.',
  accountHint: 'You only need to pair once. Come back here to review or remove connected devices.',

  pairTitle: 'Connect to your laptop',
  pairIntro: 'Open TAJ FROID on your computer, choose Connect a phone, then scan the QR code or type the code shown there.',
  pairExpired: 'That connection has expired. Get a fresh QR code or pairing code from your computer and try again.',
  pairCodeLabel: 'Pairing code',
  pairCodeHelp: 'Enter the short code shown on your computer.',
  pairAdvanced: 'Advanced settings',
  pairRelayLabel: 'Relay address',
  pairRelayHelp: 'This is filled in automatically when you scan the QR code. Only change it if support asks you to.',
  pairConnecting: 'Connecting…',
  pairAutoConnecting: 'Connecting to your laptop…',
  pairSuccess: 'Connected to your laptop.',
  pairFailed: 'Could not connect. Check the code and try again.',
  pairMissingCode: 'Enter the pairing code shown on your computer.',
  pairMissingRelay: 'Scan the QR code or ask support for the advanced relay address.',

  scanTitle: 'Scan supplier paper',
  scanTakePhoto: 'Take photo',
  scanSaved: 'Photo saved on this phone. It will upload when connected.',
  scanQueueing: 'Saving photo…',

  queueTitle: 'Photos to send',
  queueHint: 'Photos stay private on this phone until they are sent to your computer.',
  queueNotConnected: 'Connect to your computer to send these photos.',
  queueEmpty: 'No photos waiting to be sent.',
  queuePairToSync: 'Pair to sync',
  queueSending: 'Sending…',
  queueSyncNow: 'Sync now',
  queueStatus: {
    processing: 'Preparing',
    pending_upload: 'Waiting to send',
    upload_failed: 'Could not send',
    uploaded: 'Sent',
    imported: 'Added to purchases',
    rejected: 'Needs review',
    failed: 'Could not finish',
    failed_local: 'Saved on this phone',
  },

  mutationTitle: 'Changes waiting to sync',
  mutationSyncing: 'Sending changes…',
  mutationCounts: (pending, conflicts) => `${pending} waiting, ${conflicts} need review`,
  mutationHint: 'Conflicts mean another device changed the same record. "Retry local" resends your change; "Keep server" discards your queued change.',

  empty: {
    catalog: 'No products to show yet. Your catalog will appear here once it syncs from the computer.',
    inventory: 'No products to show yet. Add one here or sync your inventory from the computer.',
    needsPricing: 'Everything is priced already. Nice work!',
  checkingPrices: 'Checking prices…',
    sales: 'No sales yet. Create an invoice when you make a sale.',
    purchases: 'No purchases yet. Add one when stock arrives from a supplier.',
    customers: 'No customers yet. Add your first customer to get started.',
    suppliers: 'No suppliers yet. Add your first supplier to get started.',
    financeDebtors: 'Nobody owes money. 🎉',
    financeChecksDue: 'No checks due this week.',
    financeChecks: 'No checks registered.',
    financeExpenses: 'No expenses recorded.',
    financeJournal: 'Nothing in the journal yet.',
  },
}

export function friendlyMessage(input) {
  if (!input) return COPY.requestFailed

  const err = typeof input === 'string' ? { message: input } : input
  const raw = String(err?.message || err?.error || input || '').trim()
  const message = raw.replace(/^error:\s*/i, '')
  const code = String(err?.code || '')

  if (code === 'PAIR_FAILED' || /pair(ing)? fail/i.test(message)) return COPY.pairFailed
  if (code === 'NOT_CONNECTED' || /relay url is not configured|not configured/i.test(message)) return COPY.notConnected
  if (code === 'OFFLINE' || /offline|failed to fetch|network|enotfound|econnrefused|timed? ?out|aborted/i.test(message)) return COPY.offlineError
  if (code === 'SESSION_EXPIRED' || /401|unauthori[sz]ed|expired|revoked|invalid token|forbidden/i.test(message)) return COPY.sessionExpired
  if (code === 'CONFLICT' || /409|conflict/i.test(message)) return COPY.conflict
  if (code === 'DUPLICATE' || /duplicate|unique constraint/i.test(message)) return COPY.duplicate
  if (/5\d\d|internal server|unavailable/i.test(message)) return COPY.serverProblem

  if (!message || /^\[object /i.test(message) || /https?:\/\/|bearer |authorization|sqlite_|sql:|relayurl/i.test(message)) {
    return COPY.requestFailed
  }
  return message
}
