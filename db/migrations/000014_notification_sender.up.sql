-- Messages between a pickup group host and users record who sent them, so the
-- recipient can identify and reply to the sender. System-generated
-- notifications leave it NULL.
ALTER TABLE public.notifications ADD COLUMN IF NOT EXISTS sender_id UUID;
ALTER TABLE public.notifications
  ADD CONSTRAINT notifications_sender_id_fkey
    FOREIGN KEY (sender_id) REFERENCES public.users(id) ON DELETE SET NULL;

-- Lets a host find the users who messaged them about a group.
CREATE INDEX IF NOT EXISTS idx_notifications_group_sender
  ON public.notifications (pickup_group_id, sender_id)
  WHERE type = 'participant_message';
