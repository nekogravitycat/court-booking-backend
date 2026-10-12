-- Creating a pickup group now grants the pickup host role. Backfill users who
-- already hosted a group so they keep the role they would have received.
INSERT INTO public.pickup_hosts (user_id)
SELECT DISTINCT host_id FROM public.pickup_groups
ON CONFLICT (user_id) DO NOTHING;

-- A host message may reach every participant of a group, which can exceed 100.
ALTER TABLE public.manual_notification_sends
  DROP CONSTRAINT manual_notification_sends_recipient_count_check,
  ADD CONSTRAINT manual_notification_sends_recipient_count_check
    CHECK (recipient_count BETWEEN 1 AND 1000);
