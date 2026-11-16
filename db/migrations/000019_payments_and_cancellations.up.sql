-- Transfer reports (student-reported bank transfers reviewed by the coach) and
-- cancellation requests (cancel / refund / make-up records).

-- A refund can be recorded once money was received.
ALTER TABLE public.enrollments DROP CONSTRAINT IF EXISTS enrollments_payment_status_check;
ALTER TABLE public.enrollments ADD CONSTRAINT enrollments_payment_status_check
  CHECK (payment_status IN ('not_required', 'unpaid', 'reported', 'paid', 'refunded'));

-- =========================================================
-- Table: transfer_reports
-- Purpose: A student's "I transferred, last 5 digits are ..." message. An
--          unreviewed report never marks the enrollment as paid.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.transfer_reports (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  enrollment_id  UUID NOT NULL,
  last5          TEXT NOT NULL,
  amount         INTEGER NOT NULL,
  transferred_at TIMESTAMPTZ,
  note           TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL DEFAULT 'pending',
  review_note    TEXT,
  reviewed_by    UUID,
  reviewed_at    TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT transfer_reports_last5_check CHECK (last5 ~ '^[0-9]{5}$'),
  CONSTRAINT transfer_reports_amount_check CHECK (amount BETWEEN 1 AND 2000000),
  CONSTRAINT transfer_reports_status_check CHECK (status IN ('pending', 'approved', 'rejected')),
  CONSTRAINT transfer_reports_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT transfer_reports_reviewed_by_fkey
    FOREIGN KEY (reviewed_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- At most one report awaits review per enrollment.
CREATE UNIQUE INDEX IF NOT EXISTS uq_transfer_reports_pending
  ON public.transfer_reports (enrollment_id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_transfer_reports_enrollment ON public.transfer_reports (enrollment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transfer_reports_status ON public.transfer_reports (status, created_at);

-- =========================================================
-- Table: cancellation_requests
-- Purpose: Cancel / no-show / refund records. Refunds are recorded, never
--          computed or executed: the coach is the payee and handles money.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.cancellation_requests (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  enrollment_id   UUID NOT NULL,
  session_id      UUID,                       -- Set when the request concerns one session (e.g. coach no-show)
  requested_by    UUID,
  requester_role  TEXT NOT NULL,
  reason          TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',
  resolution      TEXT,
  refund_amount   INTEGER,
  resolution_note TEXT,
  resolved_by     UUID,
  resolved_at     TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT cancellation_requests_role_check CHECK (requester_role IN ('student', 'coach', 'system')),
  CONSTRAINT cancellation_requests_status_check CHECK (status IN ('pending', 'resolved')),
  CONSTRAINT cancellation_requests_resolution_check
    CHECK (resolution IS NULL OR resolution IN ('cancelled', 'refunded', 'makeup', 'rejected', 'other')),
  CONSTRAINT cancellation_requests_refund_check CHECK (refund_amount IS NULL OR refund_amount >= 0),
  CONSTRAINT cancellation_requests_enrollment_id_fkey
    FOREIGN KEY (enrollment_id) REFERENCES public.enrollments(id) ON DELETE CASCADE,
  CONSTRAINT cancellation_requests_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES public.course_sessions(id) ON DELETE SET NULL,
  CONSTRAINT cancellation_requests_requested_by_fkey
    FOREIGN KEY (requested_by) REFERENCES public.users(id) ON DELETE SET NULL,
  CONSTRAINT cancellation_requests_resolved_by_fkey
    FOREIGN KEY (resolved_by) REFERENCES public.users(id) ON DELETE SET NULL
);

-- At most one open request per enrollment and scope (whole enrollment or one session).
CREATE UNIQUE INDEX IF NOT EXISTS uq_cancellation_requests_pending
  ON public.cancellation_requests (enrollment_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'::uuid))
  WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_cancellation_requests_enrollment ON public.cancellation_requests (enrollment_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cancellation_requests_status ON public.cancellation_requests (status, created_at);
