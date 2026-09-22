CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  platform text NOT NULL CHECK (platform IN ('ios', 'android')),
  push_token text,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz
);

CREATE TYPE transfer_kind AS ENUM ('file', 'photo', 'link', 'text', 'clipboard');
CREATE TYPE transfer_status AS ENUM ('created', 'uploading', 'ready', 'downloading', 'completed', 'failed');

CREATE TABLE IF NOT EXISTS transfers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_device_id uuid NOT NULL REFERENCES devices(id),
  destination_device_id uuid NOT NULL REFERENCES devices(id),
  kind transfer_kind NOT NULL,
  status transfer_status NOT NULL DEFAULT 'created',
  display_name text,
  content_type text,
  object_key text,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  sha256 char(64) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS transfers_user_created_idx ON transfers(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS transfers_destination_status_idx ON transfers(destination_device_id, status);
