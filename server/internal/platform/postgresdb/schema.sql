CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
  id text PRIMARY KEY,
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  platform text NOT NULL CHECK (platform IN ('ios', 'android')),
  push_token text,
  created_at timestamptz NOT NULL,
  last_seen_at timestamptz
);

DO $$
BEGIN
  CREATE TYPE transfer_kind AS ENUM ('file', 'photo', 'link', 'text', 'clipboard');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END
$$;

DO $$
BEGIN
  CREATE TYPE transfer_status AS ENUM ('created', 'uploading', 'ready', 'downloading', 'completed', 'failed');
EXCEPTION
  WHEN duplicate_object THEN NULL;
END
$$;

CREATE TABLE IF NOT EXISTS transfers (
  id text PRIMARY KEY,
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  source_device_id text NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
  destination_device_id text NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
  kind transfer_kind NOT NULL,
  status transfer_status NOT NULL,
  display_name text,
  content_type text,
  object_key text,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  sha256 char(64) NOT NULL CHECK (sha256 ~ '^[a-fA-F0-9]{64}$'),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS devices_user_created_idx
  ON devices(user_id, created_at);

CREATE INDEX IF NOT EXISTS transfers_user_created_idx
  ON transfers(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS transfers_destination_status_idx
  ON transfers(destination_device_id, status);
