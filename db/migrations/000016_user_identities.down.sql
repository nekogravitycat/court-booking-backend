-- Restoring NOT NULL fails while accounts without an email or password exist
-- (for example LINE-only accounts); remove or complete them before rolling back.
DROP TABLE IF EXISTS public.user_identities;

ALTER TABLE public.users DROP CONSTRAINT IF EXISTS users_password_requires_email;

ALTER TABLE public.users
  ALTER COLUMN email SET NOT NULL,
  ALTER COLUMN password_hash SET NOT NULL;
