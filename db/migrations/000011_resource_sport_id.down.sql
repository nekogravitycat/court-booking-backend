DROP INDEX IF EXISTS public.idx_resources_sport_id;
ALTER TABLE public.resources
  DROP CONSTRAINT IF EXISTS resources_sport_id_fkey,
  DROP COLUMN IF EXISTS sport_id;
