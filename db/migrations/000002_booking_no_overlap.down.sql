-- Reverse of 000002: drop the overlap exclusion constraint.
-- The btree_gist extension is left in place; it is harmless and may be relied
-- upon by other objects.
ALTER TABLE public.bookings DROP CONSTRAINT IF EXISTS bookings_no_overlap;
