-- Booking series (seasonal rentals): one parent row per series, bookings link back to it.
CREATE TABLE public.booking_series (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES public.users(id),
  resource_id uuid NOT NULL REFERENCES public.resources(id) ON DELETE RESTRICT,
  term_months integer NOT NULL CHECK (term_months IN (3, 6, 12)),
  created_at  timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.bookings
  ADD COLUMN booking_series_id uuid REFERENCES public.booking_series(id) ON DELETE RESTRICT;

CREATE INDEX idx_bookings_booking_series_id
  ON public.bookings (booking_series_id) WHERE booking_series_id IS NOT NULL;

-- Location booking window for ordinary (non-series) bookings. Defaults keep the
-- behaviour that existed before the columns were added (no notice, 90 days ahead).
ALTER TABLE public.locations
  ADD COLUMN minimum_booking_notice_minutes integer NOT NULL DEFAULT 0
    CONSTRAINT locations_min_notice_non_negative CHECK (minimum_booking_notice_minutes >= 0),
  ADD COLUMN maximum_booking_advance_days integer NOT NULL DEFAULT 90
    CONSTRAINT locations_max_advance_range CHECK (maximum_booking_advance_days BETWEEN 1 AND 90),
  ADD CONSTRAINT locations_notice_within_advance
    CHECK (minimum_booking_notice_minutes <= maximum_booking_advance_days * 1440);

-- Pickup group series: groups created together by one batch request.
CREATE TABLE public.pickup_group_series (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  host_id    uuid NOT NULL REFERENCES public.users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE public.pickup_groups
  ADD COLUMN pickup_group_series_id uuid REFERENCES public.pickup_group_series(id) ON DELETE RESTRICT,
  ADD COLUMN social text CONSTRAINT pickup_groups_social_length CHECK (char_length(social) <= 500);

CREATE INDEX idx_pickup_groups_series_id
  ON public.pickup_groups (pickup_group_series_id) WHERE pickup_group_series_id IS NOT NULL;

-- Attendance: only absences are recorded (NULL = not marked).
ALTER TABLE public.pickup_orders
  ADD COLUMN attendance_status text CONSTRAINT pickup_orders_attendance_status_check CHECK (attendance_status IN ('absent')),
  ADD COLUMN attendance_marked_by uuid REFERENCES public.users(id),
  ADD COLUMN attendance_marked_at timestamptz;
