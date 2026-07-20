-- Reverse of 000004.
-- Structural rollback only; data dropped by the up migration is not restored.

-- users.username
ALTER TABLE public.users DROP CONSTRAINT IF EXISTS users_username_key;
ALTER TABLE public.users DROP COLUMN IF EXISTS username;

-- pickup_groups: drop the FK columns.
ALTER TABLE public.pickup_groups
  DROP CONSTRAINT IF EXISTS pickup_groups_sport_id_fkey,
  DROP CONSTRAINT IF EXISTS pickup_groups_skill_level_id_fkey;

DROP INDEX IF EXISTS idx_pickup_groups_sport_id;
DROP INDEX IF EXISTS idx_pickup_groups_skill_level_id;

ALTER TABLE public.pickup_groups
  DROP COLUMN IF EXISTS sport_id,
  DROP COLUMN IF EXISTS skill_level_id;

-- Recreate the previous skill-level enum + host snapshot columns (with defaults
-- so the rollback succeeds even if rows exist; original data is not restored).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'pickup_skill_level') THEN
        CREATE TYPE pickup_skill_level AS ENUM ('A', 'B', 'C', 'D');
    END IF;
END$$;

ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS host_name TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS host_phone TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS skill_level pickup_skill_level NOT NULL DEFAULT 'A';

-- Drop the lookup tables (skill_levels first due to FK).
DROP TABLE IF EXISTS public.skill_levels;
DROP TABLE IF EXISTS public.sports;
