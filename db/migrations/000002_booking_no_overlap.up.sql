-- Migration 000002: prevent overlapping bookings at the database level.
--
-- The application performs an overlap pre-check before inserting a booking, but
-- that check and the insert are not atomic: two concurrent requests can both
-- pass the pre-check and create overlapping (double-booked) reservations
-- (a classic TOCTOU race). This exclusion constraint makes the database the
-- final authority, rejecting any insert/update that would overlap an existing
-- non-cancelled booking for the same resource.
--
-- The range is built with tstzrange(start_time, end_time), whose default '[)'
-- bounds (inclusive start, exclusive end) match the application's strict
-- overlap semantics, so back-to-back bookings (end == next start) are allowed.
-- Cancelled bookings are excluded, mirroring HasOverlap which ignores the
-- 'cancelled' status.

-- btree_gist provides the equality operator class needed to mix the scalar
-- resource_id (WITH =) with the range overlap operator (WITH &&) in one GiST
-- exclusion constraint.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE public.bookings
  ADD CONSTRAINT bookings_no_overlap
  EXCLUDE USING gist (
    resource_id WITH =,
    tstzrange(start_time, end_time) WITH &&
  ) WHERE (status <> 'cancelled');
