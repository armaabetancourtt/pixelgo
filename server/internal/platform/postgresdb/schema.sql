CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
  token_hash char(64) PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  family_id text NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS refresh_tokens_family_idx
  ON refresh_tokens(family_id);

CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx
  ON refresh_tokens(user_id, created_at DESC);

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

CREATE TABLE IF NOT EXISTS notification_outbox (
  id bigserial PRIMARY KEY,
  device_id text NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  transfer_id text NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
  event_type text NOT NULL,
  payload jsonb NOT NULL,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  available_at timestamptz NOT NULL DEFAULT now(),
  locked_at timestamptz,
  sent_at timestamptz,
  failed_at timestamptz,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (device_id, transfer_id, event_type)
);

CREATE INDEX IF NOT EXISTS notification_outbox_pending_idx
  ON notification_outbox(available_at, id)
  WHERE sent_at IS NULL AND failed_at IS NULL;

CREATE OR REPLACE FUNCTION pixelgo_enqueue_transfer_ready_notification()
RETURNS trigger AS $
BEGIN
  IF NEW.status = 'ready'::transfer_status
     AND OLD.status IS DISTINCT FROM NEW.status THEN
    INSERT INTO notification_outbox (
      device_id,
      transfer_id,
      event_type,
      payload
    )
    SELECT
      NEW.destination_device_id,
      NEW.id,
      'transfer.ready',
      jsonb_build_object(
        'transferId', NEW.id,
        'kind', NEW.kind::text,
        'displayName', COALESCE(NEW.display_name, '')
      )
    FROM devices d
    WHERE d.id = NEW.destination_device_id
      AND d.push_token IS NOT NULL
      AND d.push_token <> ''
    ON CONFLICT (device_id, transfer_id, event_type) DO NOTHING;
  END IF;

  RETURN NEW;
END;
$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS transfers_ready_notification_outbox ON transfers;

CREATE TRIGGER transfers_ready_notification_outbox
AFTER UPDATE OF status ON transfers
FOR EACH ROW
EXECUTE FUNCTION pixelgo_enqueue_transfer_ready_notification();
