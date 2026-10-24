-- Migration 000016: external login identities (LINE) alongside email/password.
--
--   * users.email and users.password_hash become nullable so an account created
--     through an external provider needs neither. Postgres UNIQUE allows many
--     NULLs, so email uniqueness is unchanged.
--   * A password login still needs an email to log in with, so a password hash
--     without an email is rejected.
--   * user_identities maps (provider, provider_subject) to a user. A user holds
--     at most one identity per provider.
ALTER TABLE public.users
  ALTER COLUMN email DROP NOT NULL,
  ALTER COLUMN password_hash DROP NOT NULL;

ALTER TABLE public.users
  ADD CONSTRAINT users_password_requires_email
    CHECK (password_hash IS NULL OR email IS NOT NULL);

CREATE TABLE IF NOT EXISTS public.user_identities (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          UUID NOT NULL,
  provider         TEXT NOT NULL CHECK (provider IN ('line')),
  provider_subject TEXT NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

  CONSTRAINT user_identities_provider_subject_key UNIQUE (provider, provider_subject),
  CONSTRAINT user_identities_user_provider_key UNIQUE (user_id, provider),
  CONSTRAINT user_identities_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);
