-- Courses, sessions and enrollments for the coaching module.

-- =========================================================
-- Table: courses
-- Purpose: A trial, single, or recurring course owned by one coach or one
--          coach group. Students enroll in the course (not in a coach's time).
-- =========================================================
CREATE TABLE IF NOT EXISTS public.courses (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  coach_id              UUID,                                  -- Owner when the course is an individual coach's
  group_id              UUID,                                  -- Owner when the course belongs to a coach group
  created_by            UUID,
  title                 TEXT NOT NULL,
  description           TEXT NOT NULL DEFAULT '',
  teaching_content      TEXT NOT NULL DEFAULT '',
  sport_id              UUID NOT NULL,
  course_type           TEXT NOT NULL,
  capacity              INTEGER NOT NULL,
  venue_name            TEXT NOT NULL DEFAULT '',              -- Free text: coaches may bring their own venue
  location_id           UUID,                                  -- Optional link to a known location (informational)
  timezone              TEXT NOT NULL DEFAULT 'Asia/Taipei',
  price                 INTEGER NOT NULL DEFAULT 0,            -- Course fee per student (0 = free)
  venue_fee             INTEGER NOT NULL DEFAULT 0,            -- Venue fee per student, added to the transfer amount
  payment_methods       TEXT[] NOT NULL DEFAULT '{}',          -- Subset of {onsite, transfer}
  bank_code             TEXT,
  bank_account          TEXT,
  bank_account_name     TEXT,

  -- Structured rules enforced by the system.
  allow_leave           BOOLEAN NOT NULL DEFAULT false,
  leave_deadline_hours  INTEGER,                               -- Hours before a session after which leave is refused
  allow_makeup          BOOLEAN NOT NULL DEFAULT false,
  makeup_limit          INTEGER,                               -- Max make-up sessions per enrollment (NULL = unlimited)
  makeup_valid_days     INTEGER,                               -- Days after the missed session the make-up stays valid (NULL = no limit)
  allow_cancel          BOOLEAN NOT NULL DEFAULT false,
  cancel_deadline_hours INTEGER,                               -- Hours before the first session after which cancel is refused

  -- Free-text rules shown to students; never evaluated by the system.
  rules_leave           TEXT NOT NULL DEFAULT '',
  rules_makeup          TEXT NOT NULL DEFAULT '',
  rules_cancellation    TEXT NOT NULL DEFAULT '',
  rules_refund          TEXT NOT NULL DEFAULT '',
  rules_noshow          TEXT NOT NULL DEFAULT '',

  enrollment_deadline   TIMESTAMPTZ,
  status                TEXT NOT NULL DEFAULT 'open',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT courses_owner_check
    CHECK ((coach_id IS NOT NULL) <> (group_id IS NOT NULL)),
  CONSTRAINT courses_type_check
    CHECK (course_type IN ('trial', 'single', 'recurring')),
  CONSTRAINT courses_status_check
    CHECK (status IN ('open', 'closed', 'cancelled')),
  CONSTRAINT courses_capacity_check CHECK (capacity BETWEEN 1 AND 500),
  CONSTRAINT courses_price_check CHECK (price BETWEEN 0 AND 1000000),
  CONSTRAINT courses_venue_fee_check CHECK (venue_fee BETWEEN 0 AND 1000000),
  CONSTRAINT courses_payment_methods_check
    CHECK (payment_methods <@ ARRAY['onsite', 'transfer']::text[]),
  CONSTRAINT courses_transfer_bank_check
    CHECK (NOT ('transfer' = ANY (payment_methods)) OR (bank_code IS NOT NULL AND bank_account IS NOT NULL)),
  CONSTRAINT courses_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE,
  CONSTRAINT courses_group_id_fkey
    FOREIGN KEY (group_id) REFERENCES public.coach_groups(id) ON DELETE CASCADE,
  CONSTRAINT courses_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL,
  CONSTRAINT courses_sport_id_fkey
    FOREIGN KEY (sport_id) REFERENCES public.sports(id) ON DELETE RESTRICT,
  CONSTRAINT courses_location_id_fkey
    FOREIGN KEY (location_id) REFERENCES public.locations(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_courses_coach ON public.courses (coach_id);
CREATE INDEX IF NOT EXISTS idx_courses_group ON public.courses (group_id);
CREATE INDEX IF NOT EXISTS idx_courses_sport_status ON public.courses (sport_id, status);

-- Teaching coaches of a course (always includes the owner coach for individual courses).
CREATE TABLE IF NOT EXISTS public.course_coaches (
  course_id UUID NOT NULL,
  coach_id  UUID NOT NULL,
  PRIMARY KEY (course_id, coach_id),
  CONSTRAINT course_coaches_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE,
  CONSTRAINT course_coaches_coach_id_fkey
    FOREIGN KEY (coach_id) REFERENCES public.coaches(user_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_course_coaches_coach ON public.course_coaches (coach_id);

-- =========================================================
-- Table: course_sessions
-- Purpose: One row per class meeting. Recurring schedules are expanded into
--          rows at creation, so cancel / reschedule / attendance work per meeting.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.course_sessions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id     UUID NOT NULL,
  seq           INTEGER NOT NULL,
  start_at      TIMESTAMPTZ NOT NULL,
  end_at        TIMESTAMPTZ NOT NULL,
  status        TEXT NOT NULL DEFAULT 'scheduled',
  venue_note    TEXT NOT NULL DEFAULT '',
  cancel_reason TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT course_sessions_status_check CHECK (status IN ('scheduled', 'cancelled')),
  CONSTRAINT course_sessions_time_check CHECK (end_at > start_at),
  CONSTRAINT course_sessions_seq_unique UNIQUE (course_id, seq),
  CONSTRAINT course_sessions_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_course_sessions_course_start ON public.course_sessions (course_id, start_at);
CREATE INDEX IF NOT EXISTS idx_course_sessions_start ON public.course_sessions (start_at);

-- =========================================================
-- Table: enrollments
-- Purpose: A student's seat in a course. Enrollment state and payment state are
--          independent. Amounts are snapshots taken at enrollment time.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.enrollments (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id      UUID NOT NULL,
  user_id        UUID NOT NULL,
  status         TEXT NOT NULL DEFAULT 'active',
  payment_status TEXT NOT NULL,
  payment_method TEXT,
  price          INTEGER NOT NULL,
  venue_fee      INTEGER NOT NULL,
  amount_due     INTEGER NOT NULL,
  cancel_source  TEXT,
  cancelled_at   TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT enrollments_status_check CHECK (status IN ('active', 'cancelled')),
  CONSTRAINT enrollments_payment_status_check
    CHECK (payment_status IN ('not_required', 'unpaid', 'reported', 'paid')),
  CONSTRAINT enrollments_payment_method_check
    CHECK (payment_method IS NULL OR payment_method IN ('onsite', 'transfer')),
  CONSTRAINT enrollments_amount_check CHECK (amount_due = price + venue_fee),
  CONSTRAINT enrollments_course_id_fkey
    FOREIGN KEY (course_id) REFERENCES public.courses(id) ON DELETE CASCADE,
  CONSTRAINT enrollments_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- A student holds at most one active enrollment per course; cancelled ones are
-- kept as history and do not block re-enrolling.
CREATE UNIQUE INDEX IF NOT EXISTS uq_enrollments_active
  ON public.enrollments (course_id, user_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_enrollments_user ON public.enrollments (user_id, status);
CREATE INDEX IF NOT EXISTS idx_enrollments_course ON public.enrollments (course_id, status);
