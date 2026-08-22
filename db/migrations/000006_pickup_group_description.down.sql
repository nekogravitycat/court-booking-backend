-- Revert 000006: remove the pickup group description field.
ALTER TABLE public.pickup_groups
  DROP COLUMN IF EXISTS description;
