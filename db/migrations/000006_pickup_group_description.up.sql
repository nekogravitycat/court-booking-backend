-- Migration 000006: add a description field to pickup groups.
--
-- Rationale:
--   * Lets a host provide free-form details about the activity (ball type,
--     number of courts, etc). Capped at 100 characters, enforced at the
--     application layer.
ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS description TEXT;
