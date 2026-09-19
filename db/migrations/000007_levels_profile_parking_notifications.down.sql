-- Reverse of 000007.
-- Structural rollback only; data that has no place in the old schema (parties,
-- ratings, notifications, demographics, parking) is dropped.

DROP TABLE IF EXISTS public.notifications;
DROP TABLE IF EXISTS public.skill_ratings;
DROP TABLE IF EXISTS public.pickup_order_members;

ALTER TABLE public.pickup_orders
  DROP CONSTRAINT IF EXISTS pickup_orders_skill_level_positive,
  DROP CONSTRAINT IF EXISTS pickup_orders_party_size_positive,
  DROP COLUMN IF EXISTS skill_level,
  DROP COLUMN IF EXISTS party_size;

-- skill_levels: restore name / sort_order.
ALTER TABLE public.skill_levels
  ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;
UPDATE public.skill_levels SET sort_order = level;
ALTER TABLE public.skill_levels
  RENAME CONSTRAINT skill_levels_sport_label_unique TO skill_levels_sport_name_unique;
ALTER TABLE public.skill_levels RENAME COLUMN label TO name;

-- Make sure every (sport, level) used by a group has a mapping row to point at.
INSERT INTO public.skill_levels (sport_id, level, name, sort_order)
SELECT DISTINCT pg.sport_id, pg.skill_level, 'Level ' || pg.skill_level, pg.skill_level
FROM public.pickup_groups pg
ON CONFLICT DO NOTHING;

-- pickup_groups: restore the skill_level_id FK.
ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS skill_level_id UUID;

UPDATE public.pickup_groups pg
  SET skill_level_id = sl.id
  FROM public.skill_levels sl
  WHERE sl.sport_id = pg.sport_id AND sl.level = pg.skill_level;

ALTER TABLE public.pickup_groups
  ALTER COLUMN skill_level_id SET NOT NULL,
  ADD CONSTRAINT pickup_groups_skill_level_id_fkey
    FOREIGN KEY (skill_level_id) REFERENCES public.skill_levels(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_pickup_groups_skill_level_id
  ON public.pickup_groups (skill_level_id);

DROP INDEX IF EXISTS public.idx_pickup_groups_skill_level;
ALTER TABLE public.pickup_groups
  DROP CONSTRAINT IF EXISTS pickup_groups_skill_level_positive,
  DROP COLUMN skill_level;

ALTER TABLE public.skill_levels
  DROP CONSTRAINT IF EXISTS skill_levels_level_positive,
  DROP CONSTRAINT IF EXISTS skill_levels_sport_level_unique,
  DROP COLUMN level;

ALTER TABLE public.locations
  DROP CONSTRAINT IF EXISTS locations_parking_all_or_none,
  DROP CONSTRAINT IF EXISTS locations_parking_latitude_range,
  DROP CONSTRAINT IF EXISTS locations_parking_longitude_range,
  DROP COLUMN IF EXISTS parking_name,
  DROP COLUMN IF EXISTS parking_latitude,
  DROP COLUMN IF EXISTS parking_longitude;

ALTER TABLE public.users
  DROP CONSTRAINT IF EXISTS users_gender_valid,
  DROP COLUMN IF EXISTS gender,
  DROP COLUMN IF EXISTS birth_date;
