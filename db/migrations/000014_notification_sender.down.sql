DROP INDEX IF EXISTS public.idx_notifications_group_sender;
ALTER TABLE public.notifications DROP CONSTRAINT IF EXISTS notifications_sender_id_fkey;
ALTER TABLE public.notifications DROP COLUMN IF EXISTS sender_id;
