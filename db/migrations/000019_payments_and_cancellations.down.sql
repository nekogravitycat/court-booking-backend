DROP TABLE IF EXISTS public.cancellation_requests;
DROP TABLE IF EXISTS public.transfer_reports;
UPDATE public.enrollments SET payment_status = 'paid' WHERE payment_status = 'refunded';
ALTER TABLE public.enrollments DROP CONSTRAINT IF EXISTS enrollments_payment_status_check;
ALTER TABLE public.enrollments ADD CONSTRAINT enrollments_payment_status_check
  CHECK (payment_status IN ('not_required', 'unpaid', 'reported', 'paid'));
