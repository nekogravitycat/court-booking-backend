-- Public coach reviews, complaints, the internal reputation ledger, and suspensions.

-- =========================================================
-- Table: coach_reviews
-- Purpose: Public reviews of a coach by students who attended. Ratings are
--          stored in half points (0-10) so 0.5 steps stay exact integers.
--          The internal reputation score is a separate ledger.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.coach_reviews (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  enrollment_id UUID NOT NULL,
  coach_id      UUID NOT NULL,
  course_id     UUID NOT NULL,
  reviewer_id   UUID NOT NULL,
  rating_x2     SMALLINT NOT NULL,
  comment       TEXT NOT NULL DEFAULT '',
  is_visible    BOOLEAN NOT NULL DEFAULT true,
  hidden_reason TEXT,
  hidden_by     UUID,
  hidden_at     TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT coach_reviews_rating_check CHECK (rating_x2 BETWEEN 0 AND 10),
  CONSTRAINT coach_reviews_comment_check CHECK (char_length(comment) <= 50),
  CONSTRAINT coach_reviews_unique UNIQUE (enrollment_id, coach_id),
  CONSTRAINT coach_reviews_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT coach_reviews_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_reviews_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE,
  CONSTRAINT coach_reviews_reviewer_id_fkey
    FOREIGN KEY (reviewer_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT coach_reviews_hidden_by_fkey
    FOREIGN KEY (hidden_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_coach_reviews_coach ON public.coach_reviews (coach_id, is_visible, created_at DESC);

-- =========================================================
-- Table: complaints
-- Purpose: A simple ticket: a student complains about a coach, or a coach about
--          a student, about one enrollment. More workflow can be layered on via
--          complaint_events and new statuses.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.complaints (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  reporter_id     UUID NOT NULL,
  target_user_id  UUID NOT NULL,
  enrollment_id   UUID NOT NULL,
  category        TEXT NOT NULL,
  description     TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'open',
  resolution_note TEXT,
  resolved_by     UUID,
  resolved_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT complaints_status_check CHECK (status IN ('open', 'in_review', 'resolved', 'dismissed')),
  CONSTRAINT complaints_description_check CHECK (char_length(description) BETWEEN 1 AND 1000),
  CONSTRAINT complaints_reporter_id_fkey
    FOREIGN KEY (reporter_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT complaints_target_user_id_fkey
    FOREIGN KEY (target_user_id) REFERENCES public.users(id) ON DELETE CASCADE,
  CONSTRAINT complaints_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT complaints_resolved_by_fkey
    FOREIGN KEY (resolved_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- One live complaint per reporter, target and enrollment keeps the queue clean.
CREATE UNIQUE INDEX IF NOT EXISTS uq_complaints_live
  ON public.complaints (reporter_id, target_user_id, enrollment_id) WHERE status IN ('open', 'in_review');
CREATE INDEX IF NOT EXISTS idx_complaints_status ON public.complaints (status, created_at);
CREATE INDEX IF NOT EXISTS idx_complaints_target ON public.complaints (target_user_id, status);

-- Append-only history: status changes (visible to the reporter) and internal notes.
CREATE TABLE IF NOT EXISTS public.complaint_events (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  complaint_id  UUID NOT NULL,
  actor_id      UUID,
  event_type    TEXT NOT NULL,
  from_status   TEXT,
  to_status     TEXT,
  note          TEXT NOT NULL DEFAULT '',
  is_internal   BOOLEAN NOT NULL DEFAULT false,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT complaint_events_type_check CHECK (event_type IN ('created', 'status_changed', 'note')),
  CONSTRAINT complaint_events_complaint_id_fkey
    FOREIGN KEY (complaint_id) REFERENCES public.complaints(id) ON DELETE CASCADE,
  CONSTRAINT complaint_events_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_complaint_events_complaint ON public.complaint_events (complaint_id, created_at);

-- =========================================================
-- Table: coach_reputation_events
-- Purpose: Internal, append-only reputation ledger. No deduction rule,
--          threshold or automatic suspension exists until the platform
--          confirms one: admins record adjustments by hand.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.coach_reputation_events (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  coach_id     UUID NOT NULL,
  delta        INTEGER NOT NULL,
  reason       TEXT NOT NULL,
  complaint_id UUID,
  created_by   UUID,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT coach_reputation_events_delta_check CHECK (delta BETWEEN -100 AND 100 AND delta <> 0),
  CONSTRAINT coach_reputation_events_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_reputation_events_complaint_id_fkey
    FOREIGN KEY (complaint_id) REFERENCES public.complaints(id) ON DELETE SET NULL,
  CONSTRAINT coach_reputation_events_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_coach_reputation_events_coach ON public.coach_reputation_events (coach_id, created_at DESC);

-- =========================================================
-- Table: coach_suspensions
-- =========================================================
CREATE TABLE IF NOT EXISTS public.coach_suspensions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  coach_id    UUID NOT NULL,
  reason      TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'active',
  created_by  UUID,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  lifted_by   UUID,
  lifted_at   TIMESTAMPTZ,
  lift_note   TEXT,
  CONSTRAINT coach_suspensions_status_check CHECK (status IN ('active', 'lifted')),
  CONSTRAINT coach_suspensions_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT coach_suspensions_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL,
  CONSTRAINT coach_suspensions_lifted_by_fkey
    FOREIGN KEY (lifted_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_coach_suspensions_active
  ON public.coach_suspensions (coach_id) WHERE status = 'active';
