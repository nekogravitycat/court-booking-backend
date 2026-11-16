-- Coach profiles, coach groups and group invitations.

-- Generic reference so notifications can point at any new entity
-- (coach application, course, enrollment, complaint, ...) without a column each.
ALTER TABLE public.notifications ADD COLUMN IF NOT EXISTS ref_type TEXT;
ALTER TABLE public.notifications ADD COLUMN IF NOT EXISTS ref_id UUID;

-- =========================================================
-- Table: coaches
-- Purpose: One row per user who applied to be a coach. The user id is the
--          coach id. The row doubles as the application until approved.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.coaches (
  user_id          UUID PRIMARY KEY,
  status           TEXT NOT NULL DEFAULT 'pending',
  bio              TEXT NOT NULL,
  teaching_content TEXT NOT NULL,
  target_audience  TEXT NOT NULL,
  experience       TEXT NOT NULL DEFAULT '',
  fee_info         TEXT NOT NULL,
  photo_file_id    UUID,
  reject_reason    TEXT,
  reviewed_by      UUID,
  reviewed_at      TIMESTAMPTZ,
  applied_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT coaches_status_check
    CHECK (status IN ('pending', 'approved', 'rejected', 'suspended')),
  CONSTRAINT coaches_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT coaches_photo_file_id_fkey
    FOREIGN KEY (photo_file_id) REFERENCES public.files(id) ON DELETE SET NULL,
  CONSTRAINT coaches_reviewed_by_fkey
    FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_coaches_status ON public.coaches (status, applied_at);

CREATE TABLE IF NOT EXISTS public.coach_sports (
  coach_id UUID NOT NULL,
  sport_id UUID NOT NULL,
  PRIMARY KEY (coach_id, sport_id),
  CONSTRAINT coach_sports_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_sports_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_coach_sports_sport ON public.coach_sports (sport_id);

-- Certificates are optional attachments; "provided" is derived from their presence.
CREATE TABLE IF NOT EXISTS public.coach_certificates (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  coach_id   UUID NOT NULL,
  file_id    UUID NOT NULL,
  title      TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT coach_certificates_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_certificates_file_id_fkey
    FOREIGN KEY (file_id) REFERENCES public.files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_coach_certificates_coach ON public.coach_certificates (coach_id);

-- =========================================================
-- Table: coach_groups
-- =========================================================
CREATE TABLE IF NOT EXISTS public.coach_groups (
  id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name                   TEXT NOT NULL,
  allow_private_courses  BOOLEAN NOT NULL DEFAULT false,
  is_active              BOOLEAN NOT NULL DEFAULT true,
  created_by             UUID,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT coach_groups_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- A coach can belong to at most one group: user_id is unique across all groups.
CREATE TABLE IF NOT EXISTS public.coach_group_members (
  group_id  UUID NOT NULL,
  user_id   UUID NOT NULL,
  role      TEXT NOT NULL,
  status    TEXT NOT NULL DEFAULT 'active',
  joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (group_id, user_id),
  CONSTRAINT coach_group_members_user_unique UNIQUE (user_id),
  CONSTRAINT coach_group_members_role_check
    CHECK (role IN ('leader', 'coach', 'assistant')),
  CONSTRAINT coach_group_members_status_check
    CHECK (status IN ('active', 'inactive')),
  CONSTRAINT coach_group_members_group_id_fkey
    FOREIGN KEY (group_id) REFERENCES public.coach_groups(id) ON DELETE CASCADE,
  CONSTRAINT coach_group_members_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS public.coach_group_invitations (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id     UUID NOT NULL,
  invitee_id   UUID NOT NULL,
  role         TEXT NOT NULL,
  invited_by   UUID,
  status       TEXT NOT NULL DEFAULT 'pending',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at TIMESTAMPTZ,
  CONSTRAINT coach_group_invitations_role_check
    CHECK (role IN ('coach', 'assistant')),
  CONSTRAINT coach_group_invitations_status_check
    CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled')),
  CONSTRAINT coach_group_invitations_group_id_fkey
    FOREIGN KEY (group_id) REFERENCES public.coach_groups(id) ON DELETE CASCADE,
  CONSTRAINT coach_group_invitations_invitee_id_fkey
    FOREIGN KEY (invitee_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_group_invitations_invited_by_fkey
    FOREIGN KEY (invited_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- At most one open invitation per (group, invitee).
CREATE UNIQUE INDEX IF NOT EXISTS uq_coach_group_invitations_pending
  ON public.coach_group_invitations (group_id, invitee_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_coach_group_invitations_invitee
  ON public.coach_group_invitations (invitee_id, status);
