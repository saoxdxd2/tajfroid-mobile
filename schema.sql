-- Taj-Froid mobile relay schema (Turso / libsql)
-- Timestamps as integers (unix seconds), ids as UUID text.

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  device_token_hash TEXT NOT NULL,
  name TEXT,
  paired_at INTEGER NOT NULL,
  revoked_at INTEGER,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL,
  device_id TEXT,
  created_at INTEGER NOT NULL,
  last_used_at INTEGER
);

-- Auth runs on every relay request; point lookups keep latency flat as the
-- number of paired devices and agent tokens grows.
CREATE INDEX IF NOT EXISTS idx_devices_token_hash ON devices(device_token_hash);
CREATE INDEX IF NOT EXISTS idx_agent_tokens_token_hash ON agent_tokens(token_hash);

-- Short-lived pairing codes created by the desktop agent and consumed once by
-- the phone. A code is useless after expires_at or after used_at is set.
CREATE TABLE IF NOT EXISTS pairing_sessions (
  id TEXT PRIMARY KEY,
  code TEXT NOT NULL,
  device_id TEXT,
  expires_at INTEGER NOT NULL,
  used_at INTEGER,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS catalog_meta (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  version INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS catalog (
  sku TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  purchase_price TEXT NOT NULL DEFAULT '0',
  sale_price TEXT NOT NULL,
  vat_rate TEXT NOT NULL,
  stock INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  category TEXT,
  brand TEXT
);

CREATE TABLE IF NOT EXISTS scans (
  id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  device_id TEXT NOT NULL REFERENCES devices(id),
  captured_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'uploaded',
  blob_key TEXT,
  size_bytes INTEGER,
  sha256 TEXT,
  ciphertext_b64 TEXT,
  content_type TEXT,
  purchase_reference TEXT,
  message TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  terminal_at INTEGER
);

CREATE TABLE IF NOT EXISTS scan_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_id TEXT NOT NULL REFERENCES scans(id),
  from_status TEXT,
  to_status TEXT NOT NULL,
  at INTEGER NOT NULL,
  note TEXT
);

-- Categories
CREATE TABLE IF NOT EXISTS categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Brands
CREATE TABLE IF NOT EXISTS brands (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Customers
CREATE TABLE IF NOT EXISTS customers (
  id TEXT PRIMARY KEY,
  party_type TEXT NOT NULL DEFAULT 'enterprise',
  company_name TEXT NOT NULL,
  first_name TEXT,
  last_name TEXT,
  contact_name TEXT,
  email TEXT,
  phone TEXT,
  ice_number TEXT,
  is_archived INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Suppliers
CREATE TABLE IF NOT EXISTS suppliers (
  id TEXT PRIMARY KEY,
  party_type TEXT NOT NULL DEFAULT 'enterprise',
  company_name TEXT NOT NULL,
  first_name TEXT,
  last_name TEXT,
  contact_name TEXT,
  email TEXT,
  phone TEXT,
  ice_number TEXT,
  is_archived INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);


-- Invoices
CREATE TABLE IF NOT EXISTS invoices (
  id TEXT PRIMARY KEY,
  invoice_number TEXT NOT NULL UNIQUE,
  customer_id TEXT NOT NULL REFERENCES customers(id),
  state TEXT NOT NULL DEFAULT 'Draft',
  total_amount TEXT NOT NULL DEFAULT '0',
  channel TEXT NOT NULL DEFAULT 'store',
  due_date INTEGER,
  notes TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Invoice items
CREATE TABLE IF NOT EXISTS invoice_items (
  id TEXT PRIMARY KEY,
  invoice_id TEXT NOT NULL REFERENCES invoices(id),
  product_sku TEXT NOT NULL,
  product_name TEXT,
  quantity INTEGER NOT NULL DEFAULT 1,
  unit_price TEXT NOT NULL DEFAULT '0',
  vat_rate TEXT NOT NULL DEFAULT '20',
  unit_cost TEXT NOT NULL DEFAULT '0',
  warranty_months INTEGER NOT NULL DEFAULT 12,
  description_override TEXT,
  warranty_end_date INTEGER,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Payments
CREATE TABLE IF NOT EXISTS payments (
  id TEXT PRIMARY KEY,
  invoice_id TEXT NOT NULL REFERENCES invoices(id),
  method TEXT NOT NULL DEFAULT 'Cash',
  amount TEXT NOT NULL DEFAULT '0',
  check_id TEXT,
  reference TEXT,
  notes TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Purchases
CREATE TABLE IF NOT EXISTS purchases (
  id TEXT PRIMARY KEY,
  reference TEXT NOT NULL UNIQUE,
  supplier_id TEXT NOT NULL REFERENCES suppliers(id),
  state TEXT NOT NULL DEFAULT 'Draft',
  total_amount TEXT NOT NULL DEFAULT '0',
  purchase_date INTEGER,
  supplier_invoice_no TEXT,
  source_file_name TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS purchase_items (
  id TEXT PRIMARY KEY,
  purchase_id TEXT NOT NULL REFERENCES purchases(id),
  product_sku TEXT NOT NULL,
  product_name TEXT,
  quantity INTEGER NOT NULL,
  unit_cost TEXT NOT NULL DEFAULT '0',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Checks (physical paper, incoming from customers / outgoing to suppliers)
CREATE TABLE IF NOT EXISTS checks (
  id TEXT PRIMARY KEY,
  check_number TEXT NOT NULL,
  direction TEXT NOT NULL DEFAULT 'Incoming',
  status TEXT NOT NULL DEFAULT 'Pending',
  amount TEXT NOT NULL,
  due_date INTEGER NOT NULL,
  bank TEXT,
  party_name TEXT NOT NULL,
  customer_id TEXT,
  supplier_id TEXT,
  notes TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Expenses
CREATE TABLE IF NOT EXISTS expenses (
  id TEXT PRIMARY KEY,
  reference TEXT NOT NULL UNIQUE,
  category TEXT NOT NULL DEFAULT 'Other',
  description TEXT NOT NULL,
  amount TEXT NOT NULL,
  is_archived INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

-- Immutable financial journal
CREATE TABLE IF NOT EXISTS journal_entries (
  id TEXT PRIMARY KEY,
  transaction_type TEXT NOT NULL,
  reference_id TEXT NOT NULL,
  description TEXT NOT NULL,
  amount TEXT NOT NULL,
  is_reversed INTEGER NOT NULL DEFAULT 0,
  reversal_reference TEXT,
  created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status);
CREATE INDEX IF NOT EXISTS idx_scans_device ON scans(device_id);