-- db/init/schema.sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'organization_role') THEN
        CREATE TYPE organization_role AS ENUM ('owner', 'admin', 'member');
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS public.organizations (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS public.announcements (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  title      TEXT NOT NULL,
  content    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS public.users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name  TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ,
  is_active     BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE IF NOT EXISTS public.locations (
  id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  organization_id      BIGINT NOT NULL,
  capacity             BIGINT NOT NULL,
  opening_hours_start  TIME WITHOUT TIME ZONE NOT NULL,
  opening_hours_end    TIME WITHOUT TIME ZONE NOT NULL,
  location_info        TEXT NOT NULL,
  opening              BOOLEAN NOT NULL,
  rule                 TEXT NOT NULL,
  facility             TEXT NOT NULL,
  description          TEXT NOT NULL,
  longitude            NUMERIC NOT NULL CHECK (longitude >= '-180.0'::NUMERIC AND longitude <= 180.0),
  latitude             NUMERIC NOT NULL CHECK (latitude >= '-90.0'::NUMERIC AND latitude <= 90.0),
  CONSTRAINT locations_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id)
);

CREATE TABLE IF NOT EXISTS public.resource_types (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  organization_id BIGINT NOT NULL,
  CONSTRAINT resource_types_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id)
);

CREATE TABLE IF NOT EXISTS public.resources (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  resource_type_id BIGINT NOT NULL,
  CONSTRAINT resources_resource_type_id_fkey
    FOREIGN KEY (resource_type_id) REFERENCES public.resource_types(id)
);

CREATE TABLE IF NOT EXISTS public.organization_permissions (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id BIGINT NOT NULL,
  user_id         UUID NOT NULL,
  role            organization_role NOT NULL,
  CONSTRAINT organization_permissions_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id),
  CONSTRAINT organization_permissions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_permissions_org_user
  ON public.organization_permissions (organization_id, user_id);

CREATE INDEX IF NOT EXISTS idx_locations_org
  ON public.locations (organization_id);

CREATE INDEX IF NOT EXISTS idx_resource_types_org
  ON public.resource_types (organization_id);

CREATE INDEX IF NOT EXISTS idx_resources_type
  ON public.resources (resource_type_id);
