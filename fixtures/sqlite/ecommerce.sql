PRAGMA foreign_keys = ON;
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  first_name TEXT NOT NULL,
  country_code TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE products (
  id TEXT PRIMARY KEY,
  sku TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  price NUMERIC NOT NULL CHECK (price > 0),
  metadata TEXT,
  slug TEXT GENERATED ALWAYS AS (lower(replace(name, ' ', '-'))) STORED
);
CREATE TABLE categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  parent_id TEXT REFERENCES categories(id) DEFERRABLE INITIALLY DEFERRED
);
CREATE TABLE addresses (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  line1 TEXT NOT NULL,
  middle_name TEXT,
  country_code TEXT NOT NULL
);
CREATE TABLE orders (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  status TEXT NOT NULL CHECK(status IN ('pending','paid','shipped')),
  created_at TEXT NOT NULL,
  UNIQUE(user_id, created_at)
);
CREATE TABLE order_items (
  order_id TEXT NOT NULL REFERENCES orders(id),
  product_id TEXT NOT NULL REFERENCES products(id),
  quantity INTEGER NOT NULL CHECK(quantity >= 1),
  price NUMERIC NOT NULL CHECK(price > 0),
  PRIMARY KEY(order_id, product_id)
);
CREATE TABLE payments (
  id TEXT PRIMARY KEY,
  order_id TEXT NOT NULL UNIQUE REFERENCES orders(id),
  amount NUMERIC NOT NULL CHECK(amount > 0),
  paid_at TEXT NOT NULL
);
