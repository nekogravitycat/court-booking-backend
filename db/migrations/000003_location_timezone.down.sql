-- Reverse of 000003: drop the location timezone column.
ALTER TABLE public.locations DROP COLUMN IF EXISTS timezone;
