-- resources: optionally associate a resource with a sport.
-- Nullable so existing resources remain valid without a backfill.
ALTER TABLE public.resources
  ADD COLUMN sport_id UUID,
  ADD CONSTRAINT resources_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_resources_sport_id
  ON public.resources (sport_id);
