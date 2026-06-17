-- Migration 000003: add an IANA timezone to locations.
--
-- opening_hours_start / opening_hours_end are stored as local wall-clock TIME
-- values, but bookings are stored as absolute TIMESTAMPTZ instants. Without a
-- per-location timezone the two cannot be compared correctly: availability and
-- opening-hours validation would treat the local hours as if they were UTC.
--
-- The column holds an IANA timezone name (e.g. 'Asia/Taipei'). Existing rows
-- default to 'UTC', which preserves the previous (UTC-based) behaviour until an
-- operator sets the correct zone. The application validates the name against
-- the Go time database on create/update.
ALTER TABLE public.locations
  ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';
