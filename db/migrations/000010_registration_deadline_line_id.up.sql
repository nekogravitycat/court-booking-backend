ALTER TABLE public.users ADD COLUMN line_id varchar(20) CHECK (line_id ~ '^[a-z0-9._-]{4,20}$');
ALTER TABLE public.pickup_groups ADD COLUMN registration_deadline timestamptz;
UPDATE public.pickup_groups SET registration_deadline = start_time;
ALTER TABLE public.pickup_groups ALTER COLUMN registration_deadline SET NOT NULL;
ALTER TABLE public.pickup_groups ADD CONSTRAINT pickup_groups_registration_deadline_check CHECK (registration_deadline <= start_time);
CREATE TABLE public.manual_notification_sends (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 sender_id uuid NOT NULL REFERENCES public.users(id),
 recipient_count integer NOT NULL CHECK (recipient_count BETWEEN 1 AND 100),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX manual_notification_sends_sender_created_idx ON public.manual_notification_sends(sender_id, created_at);
