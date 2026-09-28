-- Revert pickup_groups.min_skill_level/max_skill_level back to a single skill_level.
-- The max bound is discarded; only min_skill_level survives as skill_level.

ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS skill_level INTEGER;

UPDATE public.pickup_groups
  SET skill_level = min_skill_level;

ALTER TABLE public.pickup_groups
  ALTER COLUMN skill_level SET NOT NULL,
  ADD CONSTRAINT pickup_groups_skill_level_positive CHECK (skill_level >= 1);

CREATE INDEX IF NOT EXISTS idx_pickup_groups_skill_level
  ON public.pickup_groups (skill_level);

DROP INDEX IF EXISTS public.idx_pickup_groups_min_skill_level;
DROP INDEX IF EXISTS public.idx_pickup_groups_max_skill_level;
ALTER TABLE public.pickup_groups
  DROP CONSTRAINT IF EXISTS pickup_groups_min_skill_level_positive,
  DROP CONSTRAINT IF EXISTS pickup_groups_max_skill_level_positive,
  DROP CONSTRAINT IF EXISTS pickup_groups_skill_level_range_valid,
  DROP COLUMN IF EXISTS min_skill_level,
  DROP COLUMN IF EXISTS max_skill_level;
