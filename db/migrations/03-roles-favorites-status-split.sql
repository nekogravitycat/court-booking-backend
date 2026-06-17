-- Migration: pickup host role, favorite hosts, and status/payment_status split.
-- Date: 2026-06-17
--
-- Covers:
--   1. pickup_hosts table (new global "pickup host" role)
--   2. favorite_hosts table (favorite pickup hosts)
--   3. bookings: add cancel_request status + payment_status column
--   4. pickup_orders: add status column; rename payment 'paid' -> 'done'
--
-- NOTE: ALTER TYPE ... ADD VALUE / RENAME VALUE cannot run inside the same
--       transaction block that later uses the new value, so the enum changes
--       are split from the column changes below.

-- ---------------------------------------------------------
-- 1. pickup_hosts table
-- ---------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.pickup_hosts (
  user_id     UUID PRIMARY KEY,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT pickup_hosts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- ---------------------------------------------------------
-- 2. favorite_hosts table
-- ---------------------------------------------------------
CREATE TABLE IF NOT EXISTS public.favorite_hosts (
  user_id     UUID NOT NULL,
  host_id     UUID NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, host_id),
  CONSTRAINT favorite_hosts_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT favorite_hosts_host_id_fkey
    FOREIGN KEY (host_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_favorite_hosts_host_id
  ON public.favorite_hosts (host_id);

-- ---------------------------------------------------------
-- 3. bookings: status + payment_status
-- ---------------------------------------------------------
-- Add the new lifecycle status value (idempotent on PG 12+).
ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'cancel_request';

-- New payment status enum + column.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'booking_payment_status') THEN
        CREATE TYPE booking_payment_status AS ENUM ('done', 'pending', 'failed');
    END IF;
END
$$;

ALTER TABLE public.bookings
  ADD COLUMN IF NOT EXISTS payment_status booking_payment_status NOT NULL DEFAULT 'pending';

-- ---------------------------------------------------------
-- 4. pickup_orders: status + payment rename paid -> done
-- ---------------------------------------------------------
-- New enrollment status enum + column.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'pickup_order_status') THEN
        CREATE TYPE pickup_order_status AS ENUM ('pending', 'confirmed', 'cancelled', 'cancel_request');
    END IF;
END
$$;

ALTER TABLE public.pickup_orders
  ADD COLUMN IF NOT EXISTS status pickup_order_status NOT NULL DEFAULT 'pending';

-- Migrate legacy payment_status 'cancelled' rows into the new status column
-- before the enum value is removed.
UPDATE public.pickup_orders
  SET status = 'cancelled'
  WHERE payment_status::TEXT = 'cancelled';

-- Rename the existing 'paid' value to 'done' (PG 10+).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_enum e
        JOIN pg_type t ON t.oid = e.enumtypid
        WHERE t.typname = 'pickup_payment_status' AND e.enumlabel = 'paid'
    ) THEN
        ALTER TYPE pickup_payment_status RENAME VALUE 'paid' TO 'done';
    END IF;
END
$$;

-- Map any leftover 'cancelled' payment status to 'pending' (its value is no
-- longer meaningful now that cancellation lives on the status column).
-- The 'cancelled' enum label itself is intentionally left in place because
-- PostgreSQL cannot drop an enum value; application code no longer emits it.
UPDATE public.pickup_orders
  SET payment_status = 'pending'
  WHERE payment_status::TEXT = 'cancelled';
