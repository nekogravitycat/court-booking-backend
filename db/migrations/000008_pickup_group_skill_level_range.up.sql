-- pickup_groups: split the single skill_level into a min/max range.
-- min_skill_level is required (backfilled from the old skill_level);
-- max_skill_level is optional (unbounded above when null).

ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS min_skill_level INTEGER,
  ADD COLUMN IF NOT EXISTS max_skill_level INTEGER;

UPDATE public.pickup_groups
  SET min_skill_level = skill_level,
      max_skill_level = skill_level;

ALTER TABLE public.pickup_groups
  ALTER COLUMN min_skill_level SET NOT NULL,
  ADD CONSTRAINT pickup_groups_min_skill_level_positive CHECK (min_skill_level >= 1),
  ADD CONSTRAINT pickup_groups_max_skill_level_positive CHECK (max_skill_level IS NULL OR max_skill_level >= 1),
  ADD CONSTRAINT pickup_groups_skill_level_range_valid CHECK (max_skill_level IS NULL OR max_skill_level >= min_skill_level);

DROP INDEX IF EXISTS public.idx_pickup_groups_skill_level;
ALTER TABLE public.pickup_groups
  DROP CONSTRAINT IF EXISTS pickup_groups_skill_level_positive,
  DROP COLUMN IF EXISTS skill_level;

CREATE INDEX IF NOT EXISTS idx_pickup_groups_min_skill_level
  ON public.pickup_groups (min_skill_level);
CREATE INDEX IF NOT EXISTS idx_pickup_groups_max_skill_level
  ON public.pickup_groups (max_skill_level);
