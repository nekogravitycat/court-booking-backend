-- Attendance, leave requests and make-up sessions.

-- =========================================================
-- Table: session_attendances
-- Purpose: One row per (session, enrollment) once something is recorded:
--          a leave, an attendance mark, or a make-up booking. A regular
--          student with no row is simply "pending".
-- =========================================================
CREATE TABLE IF NOT EXISTS public.session_attendances (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id    UUID NOT NULL,
  enrollment_id UUID NOT NULL,
  kind          TEXT NOT NULL DEFAULT 'regular',      -- regular: own course; makeup: guest in another class
  status        TEXT NOT NULL DEFAULT 'pending',
  marked_by     UUID,
  marked_at     TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT session_attendances_kind_check CHECK (kind IN ('regular', 'makeup')),
  CONSTRAINT session_attendances_status_check CHECK (status IN ('pending', 'present', 'absent', 'leave')),
  CONSTRAINT session_attendances_unique UNIQUE (session_id, enrollment_id),
  CONSTRAINT session_attendances_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.course_sessions(id) ON DELETE CASCADE,
  CONSTRAINT session_attendances_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT session_attendances_marked_by_fkey
    FOREIGN KEY (marked_by) REFERENCES public.users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_session_attendances_enrollment ON public.session_attendances (enrollment_id, status);

-- =========================================================
-- Table: leave_requests
-- Purpose: A student's leave for one session, and the make-up session chosen
--          for it (if any). Leaving frees the seat for that session.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.leave_requests (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id        UUID NOT NULL,
  enrollment_id     UUID NOT NULL,
  reason            TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL DEFAULT 'open',       -- open: no make-up chosen; used: make-up booked
  makeup_session_id UUID,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT leave_requests_status_check CHECK (status IN ('open', 'used')),
  CONSTRAINT leave_requests_makeup_check CHECK ((status = 'used') = (makeup_session_id IS NOT NULL)),
  CONSTRAINT leave_requests_unique UNIQUE (session_id, enrollment_id),
  CONSTRAINT leave_requests_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.course_sessions(id) ON DELETE CASCADE,
  CONSTRAINT leave_requests_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT leave_requests_makeup_session_id_fkey
    FOREIGN KEY (makeup_session_id) REFERENCES public.course_sessions(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_leave_requests_enrollment ON public.leave_requests (enrollment_id, status);
