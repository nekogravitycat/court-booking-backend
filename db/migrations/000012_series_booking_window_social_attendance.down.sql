ALTER TABLE public.pickup_orders
  DROP COLUMN attendance_marked_at,
  DROP COLUMN attendance_marked_by,
  DROP COLUMN attendance_status;

DROP INDEX IF EXISTS public.idx_pickup_groups_series_id;
ALTER TABLE public.pickup_groups
  DROP COLUMN social,
  DROP COLUMN pickup_group_series_id;
DROP TABLE public.pickup_group_series;

ALTER TABLE public.locations
  DROP CONSTRAINT locations_notice_within_advance,
  DROP COLUMN maximum_booking_advance_days,
  DROP COLUMN minimum_booking_notice_minutes;

DROP INDEX IF EXISTS public.idx_bookings_booking_series_id;
ALTER TABLE public.bookings DROP COLUMN booking_series_id;
DROP TABLE public.booking_series;
