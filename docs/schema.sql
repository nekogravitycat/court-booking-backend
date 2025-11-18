-- db/init/schema.sql

-- Enable pgcrypto extension for UUID generation and other crypto helpers
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =========================================================
-- Enum: organization_role
-- Purpose: Role of a user within an organization
-- =========================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'organization_role') THEN
        CREATE TYPE organization_role AS ENUM ('owner', 'admin', 'member');
    END IF;
END
$$;

-- =========================================================
-- Enum: booking_status
-- Purpose: Lifecycle status for a booking
-- =========================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'booking_status') THEN
        CREATE TYPE booking_status AS ENUM ('pending', 'confirmed', 'cancelled');
    END IF;
END
$$;

-- =========================================================
-- Table: organizations
-- Purpose: Top-level entity for a company / brand / venue owner.
--          Other entities (locations, resource_types, etc.) belong to an organization.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.organizations (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Surrogate key
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  name       TEXT NOT NULL,                                   -- Organization name
  is_active  BOOLEAN NOT NULL DEFAULT true                    -- Soft delete / Suspension status
);

-- =========================================================
-- Table: announcements
-- Purpose: System or organization-related announcements / news.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.announcements (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Announcement ID
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  title      TEXT NOT NULL,                                   -- Announcement title
  content    TEXT NOT NULL                                    -- Announcement body/content
);

-- =========================================================
-- Table: users
-- Purpose: All application users (normal users, venue managers, system admins).
--          Organization-level roles are handled in organization_permissions.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.users (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(), -- User ID
  email           TEXT NOT NULL UNIQUE,                       -- Login / contact email
  password_hash   TEXT NOT NULL,                              -- Hashed password
  display_name    TEXT,                                       -- Optional display name
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),         -- Account creation time
  last_login_at   TIMESTAMPTZ,                                -- Last login timestamp
  is_active       BOOLEAN NOT NULL DEFAULT true,              -- Soft-activation flag
  is_system_admin BOOLEAN NOT NULL DEFAULT false              -- Platform-level admin (God mode)
);

-- =========================================================
-- Table: locations
-- Purpose: Physical venue locations (branches). A location belongs to one organization.
--          Resources (courts/rooms) are conceptually inside locations.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.locations (
  id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Location ID
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  organization_id      BIGINT NOT NULL,                                 -- Owning organization
  capacity             BIGINT NOT NULL,                                 -- Max capacity (e.g. people)
  opening_hours_start  TIME WITHOUT TIME ZONE NOT NULL,                 -- Daily opening time
  opening_hours_end    TIME WITHOUT TIME ZONE NOT NULL,                 -- Daily closing time
  location_info        TEXT NOT NULL,                                   -- Address / basic info
  opening              BOOLEAN NOT NULL,                                -- Whether location is open
  rule                 TEXT NOT NULL,                                   -- Rules / terms of use
  facility             TEXT NOT NULL,                                   -- Facility description
  description          TEXT NOT NULL,                                   -- General description
  longitude            NUMERIC NOT NULL CHECK (
                          longitude >= '-180.0'::NUMERIC
                          AND longitude <= 180.0
                        ),                                              -- Geo: longitude
  latitude             NUMERIC NOT NULL CHECK (
                          latitude >= '-90.0'::NUMERIC
                          AND latitude <= 90.0
                        ),                                              -- Geo: latitude
  CONSTRAINT locations_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id)
);

-- =========================================================
-- Table: resource_types
-- Purpose: Types of resources for an organization
--          (e.g. badminton court, tennis court, meeting room).
-- =========================================================
CREATE TABLE IF NOT EXISTS public.resource_types (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Resource type ID
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  organization_id BIGINT NOT NULL,                                 -- Owning organization
  CONSTRAINT resource_types_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id)
);

-- =========================================================
-- Table: resources
-- Purpose: Bookable units (actual courts/rooms), linked to a resource_type
--          and a location.
--          For example: Court A, Court B under "badminton_court" at Location X.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.resources (
  id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Resource ID
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  resource_type_id BIGINT NOT NULL,                                 -- Type of this resource
  location_id      BIGINT NOT NULL,                                 -- Physical location
  CONSTRAINT resources_resource_type_id_fkey
    FOREIGN KEY (resource_type_id) REFERENCES public.resource_types(id),
  CONSTRAINT resources_location_id_fkey
    FOREIGN KEY (location_id) REFERENCES public.locations(id)
);

-- =========================================================
-- Table: bookings
-- Purpose: Single booking for one resource (court/room) during
--          a continuous time range, made by a user.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.bookings (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Booking ID
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Creation timestamp
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Last update timestamp

  resource_id BIGINT NOT NULL,                                 -- Booked resource (court/room)
  user_id     UUID NOT NULL,                                   -- User who made the booking

  start_time  TIMESTAMPTZ NOT NULL,                            -- Booking start time (UTC)
  end_time    TIMESTAMPTZ NOT NULL,                            -- Booking end time (UTC)
  status      booking_status NOT NULL DEFAULT 'pending',       -- Booking lifecycle status

  CONSTRAINT bookings_resource_id_fkey
    FOREIGN KEY (resource_id) REFERENCES public.resources(id),
  CONSTRAINT bookings_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id),
  CONSTRAINT bookings_time_range_valid
    CHECK (end_time > start_time)                              -- Ensure valid time range
);

-- =========================================================
-- Table: organization_permissions
-- Purpose: Per-organization membership & role for a user.
--          Used to represent organization owner/admin/member.
--          Venue managers = users with role 'owner' or 'admin' for some organization.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.organization_permissions (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- Permission row ID
  organization_id BIGINT NOT NULL,                                 -- Target organization
  user_id         UUID NOT NULL,                                   -- Target user
  role            organization_role NOT NULL,                      -- Role within organization
  CONSTRAINT organization_permissions_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id),
  CONSTRAINT organization_permissions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id)
);

-- =========================================================
-- Indexes
-- =========================================================

-- Unique constraint: one membership per (organization, user)
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_permissions_org_user
  ON public.organization_permissions (organization_id, user_id);

-- Index for querying locations by organization
CREATE INDEX IF NOT EXISTS idx_locations_org
  ON public.locations (organization_id);

-- Index for querying resource types by organization
CREATE INDEX IF NOT EXISTS idx_resource_types_org
  ON public.resource_types (organization_id);

-- Index for querying resources by resource type
CREATE INDEX IF NOT EXISTS idx_resources_type
  ON public.resources (resource_type_id);

-- Index for querying resources by location
CREATE INDEX IF NOT EXISTS idx_resources_location
  ON public.resources (location_id);

-- Index for querying bookings by resource and time (for availability checks)
CREATE INDEX IF NOT EXISTS idx_bookings_resource_time
  ON public.bookings (resource_id, start_time, end_time);

-- Index for querying bookings by user (user booking history)
CREATE INDEX IF NOT EXISTS idx_bookings_user_time
  ON public.bookings (user_id, start_time);
