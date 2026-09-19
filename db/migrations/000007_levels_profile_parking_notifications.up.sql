-- Migration 000007: integer skill levels, user demographics, parking info,
-- party enrollment, skill ratings, and in-app notifications.
--
-- Rationale:
--   * Skill levels become plain integers so they can be averaged. The
--     skill_levels table turns into a per-sport mapping (level -> label); a
--     pickup group stores only the integer level.
--   * Users gain optional gender / birth_date, used for participant statistics.
--   * Locations gain an optional parking lot (name + coordinates).
--   * A pickup order may occupy several seats (party enrollment); the anonymous
--     companions are kept in pickup_order_members. Every order records the
--     enrollee's self-reported skill level.
--   * Hosts can rate participants after a group ends (skill_ratings), and the
--     per-user average forms a composite rating.
--   * A per-user notification inbox (notifications).

-- =========================================================
-- users: optional demographics
-- =========================================================
ALTER TABLE public.users
  ADD COLUMN IF NOT EXISTS gender     TEXT,
  ADD COLUMN IF NOT EXISTS birth_date DATE;

ALTER TABLE public.users
  ADD CONSTRAINT users_gender_valid
    CHECK (gender IS NULL OR gender IN ('male', 'female', 'other'));

-- =========================================================
-- locations: optional parking lot (all three columns set, or none)
-- =========================================================
ALTER TABLE public.locations
  ADD COLUMN IF NOT EXISTS parking_name      TEXT,
  ADD COLUMN IF NOT EXISTS parking_latitude  NUMERIC,
  ADD COLUMN IF NOT EXISTS parking_longitude NUMERIC;

ALTER TABLE public.locations
  ADD CONSTRAINT locations_parking_all_or_none
    CHECK (
      (parking_name IS NULL AND parking_latitude IS NULL AND parking_longitude IS NULL)
      OR
      (parking_name IS NOT NULL AND parking_latitude IS NOT NULL AND parking_longitude IS NOT NULL)
    ),
  ADD CONSTRAINT locations_parking_latitude_range
    CHECK (parking_latitude IS NULL OR (parking_latitude >= -90 AND parking_latitude <= 90)),
  ADD CONSTRAINT locations_parking_longitude_range
    CHECK (parking_longitude IS NULL OR (parking_longitude >= -180 AND parking_longitude <= 180));

-- =========================================================
-- skill_levels: becomes a per-sport (level -> label) mapping.
-- Existing rows are numbered 1..n per sport following their old sort order.
-- =========================================================
ALTER TABLE public.skill_levels
  ADD COLUMN IF NOT EXISTS level INTEGER;

UPDATE public.skill_levels sl
  SET level = r.rn
  FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY sport_id ORDER BY sort_order, created_at, id) AS rn
    FROM public.skill_levels
  ) r
  WHERE sl.id = r.id;

ALTER TABLE public.skill_levels
  ALTER COLUMN level SET NOT NULL;

ALTER TABLE public.skill_levels
  ADD CONSTRAINT skill_levels_level_positive CHECK (level >= 1),
  ADD CONSTRAINT skill_levels_sport_level_unique UNIQUE (sport_id, level);

ALTER TABLE public.skill_levels RENAME COLUMN name TO label;
ALTER TABLE public.skill_levels
  RENAME CONSTRAINT skill_levels_sport_name_unique TO skill_levels_sport_label_unique;
ALTER TABLE public.skill_levels DROP COLUMN sort_order;

-- =========================================================
-- pickup_groups: store the integer level instead of a skill_levels FK.
-- =========================================================
ALTER TABLE public.pickup_groups
  ADD COLUMN IF NOT EXISTS skill_level INTEGER;

UPDATE public.pickup_groups pg
  SET skill_level = sl.level
  FROM public.skill_levels sl
  WHERE sl.id = pg.skill_level_id;

ALTER TABLE public.pickup_groups
  ALTER COLUMN skill_level SET NOT NULL,
  ADD CONSTRAINT pickup_groups_skill_level_positive CHECK (skill_level >= 1);

ALTER TABLE public.pickup_groups
  DROP CONSTRAINT IF EXISTS pickup_groups_skill_level_id_fkey;
DROP INDEX IF EXISTS public.idx_pickup_groups_skill_level_id;
ALTER TABLE public.pickup_groups DROP COLUMN skill_level_id;

CREATE INDEX IF NOT EXISTS idx_pickup_groups_skill_level
  ON public.pickup_groups (skill_level);

-- =========================================================
-- pickup_orders: self-reported level and party size.
-- party_size is the number of seats the order occupies (1 for a single
-- enrollment). Existing orders inherit their group's level.
-- =========================================================
ALTER TABLE public.pickup_orders
  ADD COLUMN IF NOT EXISTS skill_level INTEGER,
  ADD COLUMN IF NOT EXISTS party_size  INTEGER NOT NULL DEFAULT 1;

UPDATE public.pickup_orders po
  SET skill_level = pg.skill_level
  FROM public.pickup_groups pg
  WHERE pg.id = po.pickup_group_id;

ALTER TABLE public.pickup_orders
  ALTER COLUMN skill_level SET NOT NULL,
  ADD CONSTRAINT pickup_orders_skill_level_positive CHECK (skill_level >= 1),
  ADD CONSTRAINT pickup_orders_party_size_positive CHECK (party_size >= 1);

-- =========================================================
-- Table: pickup_order_members
-- Purpose: The anonymous members of a party order (one row per seat). They have
--          no account, so they can never be rated.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.pickup_order_members (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id    UUID NOT NULL,
  gender      TEXT NOT NULL,
  skill_level INTEGER NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT pickup_order_members_order_id_fkey
    FOREIGN KEY (order_id) REFERENCES public.pickup_orders(id) ON DELETE CASCADE,
  CONSTRAINT pickup_order_members_gender_valid
    CHECK (gender IN ('male', 'female', 'other')),
  CONSTRAINT pickup_order_members_skill_level_positive
    CHECK (skill_level >= 1)
);

CREATE INDEX IF NOT EXISTS idx_pickup_order_members_order_id
  ON public.pickup_order_members (order_id);

-- =========================================================
-- Table: skill_ratings
-- Purpose: A host's skill-level rating of one participant for one pickup group.
--          The average per (user, sport) is the user's composite rating.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.skill_ratings (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  pickup_group_id UUID NOT NULL,
  user_id         UUID NOT NULL,                              -- The rated participant
  sport_id        UUID NOT NULL,                              -- Denormalized from the group
  level           INTEGER NOT NULL,
  rated_by        UUID,                                       -- The rating host / admin
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT skill_ratings_group_id_fkey
    FOREIGN KEY (pickup_group_id) REFERENCES public.pickup_groups(id) ON DELETE CASCADE,
  CONSTRAINT skill_ratings_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT skill_ratings_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE RESTRICT,
  CONSTRAINT skill_ratings_rated_by_fkey
    FOREIGN KEY (rated_by) REFERENCES public.users(id) ON DELETE SET NULL,
  CONSTRAINT skill_ratings_level_positive
    CHECK (level >= 1),

  -- One rating per participant per group (re-rating overwrites).
  UNIQUE (pickup_group_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_skill_ratings_user_sport
  ON public.skill_ratings (user_id, sport_id);

-- =========================================================
-- Table: notifications
-- Purpose: Per-user in-app notification inbox.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.notifications (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL,                              -- The recipient
  type            TEXT NOT NULL,                              -- Machine key, e.g. 'pickup_order_confirmed'
  title           TEXT NOT NULL,
  content         TEXT NOT NULL,
  pickup_group_id UUID,                                       -- Related group (optional)
  pickup_order_id UUID,                                       -- Related order (optional)
  is_read         BOOLEAN NOT NULL DEFAULT false,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT notifications_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT notifications_pickup_group_id_fkey
    FOREIGN KEY (pickup_group_id) REFERENCES public.pickup_groups(id) ON DELETE SET NULL,
  CONSTRAINT notifications_pickup_order_id_fkey
    FOREIGN KEY (pickup_order_id) REFERENCES public.pickup_orders(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_created
  ON public.notifications (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
  ON public.notifications (user_id) WHERE is_read = false;
