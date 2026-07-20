-- Migration 000004: admin-managed sport & skill-level lookup tables, pickup group
-- FK references, and a unique username on users.
--
-- Rationale:
--   * Sports and skill levels become admin-editable lookup tables so new sports
--     (and per-sport grading schemes) can be added without a schema change.
--   * pickup_groups no longer snapshots host_name/host_phone or a fixed skill
--     enum; host details are resolved live via the users table (host_id), and
--     sport / skill level are foreign keys into the new lookup tables.
--   * Users gain a Twitter-style unique username (lowercase letters, digits,
--     underscore). Existing users are backfilled with a deterministic value
--     derived from their id.

-- =========================================================
-- Table: sports
-- Purpose: Admin-managed catalog of ball sports (enum-like lookup).
-- =========================================================
CREATE TABLE IF NOT EXISTS public.sports (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code       TEXT NOT NULL UNIQUE,                     -- Stable machine key, e.g. 'BADMINTON'
  name       TEXT NOT NULL,                            -- Human-readable display name
  is_active  BOOLEAN NOT NULL DEFAULT true,            -- Soft-delete flag
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================================================
-- Table: skill_levels
-- Purpose: Admin-managed grading tiers, scoped to a single sport.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.skill_levels (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sport_id   UUID NOT NULL,
  name       TEXT NOT NULL,                            -- Grade label, e.g. 'A' / 'Beginner'
  sort_order INTEGER NOT NULL DEFAULT 0,               -- Display ordering within a sport
  is_active  BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT skill_levels_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE RESTRICT,

  CONSTRAINT skill_levels_sport_name_unique
    UNIQUE (sport_id, name)
);

CREATE INDEX IF NOT EXISTS idx_skill_levels_sport_id
  ON public.skill_levels (sport_id);

-- =========================================================
-- pickup_groups: replace host snapshots + skill enum with FK references.
-- No existing pickup group data needs to be preserved, so NOT NULL columns are
-- added directly.
-- =========================================================
ALTER TABLE public.pickup_groups
  DROP COLUMN IF EXISTS host_name,
  DROP COLUMN IF EXISTS host_phone,
  DROP COLUMN IF EXISTS skill_level,
  ADD COLUMN sport_id UUID NOT NULL,
  ADD COLUMN skill_level_id UUID NOT NULL;

ALTER TABLE public.pickup_groups
  ADD CONSTRAINT pickup_groups_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE RESTRICT,
  ADD CONSTRAINT pickup_groups_skill_level_id_fkey
    FOREIGN KEY (skill_level_id) REFERENCES public.skill_levels(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_pickup_groups_sport_id
  ON public.pickup_groups (sport_id);

CREATE INDEX IF NOT EXISTS idx_pickup_groups_skill_level_id
  ON public.pickup_groups (skill_level_id);

-- The old fixed skill-level enum type is no longer referenced by any column.
DROP TYPE IF EXISTS pickup_skill_level;

-- =========================================================
-- users: add a unique, immutable username.
-- =========================================================
ALTER TABLE public.users
  ADD COLUMN IF NOT EXISTS username TEXT;

-- Backfill existing users with a deterministic, unique, format-valid value
-- ('u' + first 14 hex chars of the id => 15 chars, matching ^[a-z0-9_]{4,15}$).
UPDATE public.users
  SET username = 'u' || substr(replace(id::text, '-', ''), 1, 14)
  WHERE username IS NULL;

ALTER TABLE public.users
  ALTER COLUMN username SET NOT NULL;

ALTER TABLE public.users
  ADD CONSTRAINT users_username_key UNIQUE (username);

-- =========================================================
-- Seed initial sports and per-sport skill levels so the system is usable
-- immediately after migration.
-- =========================================================
INSERT INTO public.sports (code, name) VALUES
  ('BADMINTON', 'Badminton'),
  ('BASKETBALL', 'Basketball'),
  ('VOLLEYBALL', 'Volleyball')
ON CONFLICT (code) DO NOTHING;

INSERT INTO public.skill_levels (sport_id, name, sort_order)
SELECT s.id, lvl.name, lvl.sort_order
FROM public.sports s
CROSS JOIN (VALUES ('A', 1), ('B', 2), ('C', 3), ('D', 4)) AS lvl(name, sort_order)
WHERE s.code IN ('BADMINTON', 'BASKETBALL', 'VOLLEYBALL')
ON CONFLICT (sport_id, name) DO NOTHING;
