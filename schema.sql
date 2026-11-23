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
  created_at INTEGER NOT NULL,
  last_used_at INTEGER
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

CREATE INDEX IF NOT EXISTS idx_scans_status ON scans(status);
CREATE INDEX IF NOT EXISTS idx_scans_device ON scans(device_id);