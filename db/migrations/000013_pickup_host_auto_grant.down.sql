-- The role backfill is not reverted: granted roles cannot be told apart from
-- roles assigned by a system administrator.
UPDATE public.manual_notification_sends SET recipient_count = 100 WHERE recipient_count > 100;
ALTER TABLE public.manual_notification_sends
  DROP CONSTRAINT manual_notification_sends_recipient_count_check,
  ADD CONSTRAINT manual_notification_sends_recipient_count_check
    CHECK (recipient_count BETWEEN 1 AND 100);
