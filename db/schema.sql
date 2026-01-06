-- db/init/schema.sql

-- Enable pgcrypto extension for UUID generation and other crypto helpers
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

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
-- Table: announcements
-- Purpose: System or organization-related announcements / news.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.announcements (
  -- Identity
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the announcement

  -- Content
  title       TEXT NOT NULL,                                   -- Headline/Title of the announcement
  content     TEXT NOT NULL,                                   -- Main body text of the announcement

  -- Meta / Audit
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the announcement was posted
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()               -- Timestamp of the last edit
);

-- =========================================================
-- Table: users
-- Purpose: All application users (normal users, venue managers, system admins).
--          Organization-level roles are handled via organizations.owner_id and organization_managers.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.users (
  -- Identity
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(), -- Unique identifier for the user

  -- Authentication & Profile
  email           TEXT NOT NULL UNIQUE,                       -- User's email address, serves as the login username
  password_hash   TEXT NOT NULL,                              -- Bcrypt/Argon2 hash of the user's password
  display_name    TEXT,                                       -- Publicly visible name (optional)

  -- Authorization
  is_system_admin BOOLEAN NOT NULL DEFAULT false,             -- "God Mode" flag: grants full access to all organizations and system settings

  -- Meta / Audit
  is_active       BOOLEAN NOT NULL DEFAULT true,              -- Status flag: false indicates a suspended or soft-deleted account
  last_login_at   TIMESTAMPTZ,                                -- Timestamp of the most recent successful login
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()          -- Timestamp when the account was registered
);

-- =========================================================
-- Table: organizations
-- Purpose: Top-level entity for a company / brand / venue owner.
--          Other entities (locations, resource_types, etc.) belong to an organization.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.organizations (
  -- Identity
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the organization

  -- Relationships
  owner_id    UUID NOT NULL,                                   -- Reference to the User who owns this organization (1 owner per org)

  -- Core Data
  name        TEXT NOT NULL,                                   -- Display name of the organization

  -- Meta / Audit
  is_active   BOOLEAN NOT NULL DEFAULT true,                   -- Status flag: false prevents all operations for this organization
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the organization was established in the system

  -- Constraint: Enforces that the 'owner_id' must correspond to a valid user in the users table.
  CONSTRAINT organizations_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES public.users(id)
);

-- =========================================================
-- Table: locations
-- Purpose: Physical venue locations (branches). A location belongs to one organization.
--          Resources (courts/rooms) are conceptually inside locations.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.locations (
  -- Identity
  id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the physical location
  
  -- Relationships
  organization_id      UUID NOT NULL,                                   -- The parent organization this location belongs to

  -- Core Settings & Status
  name                 TEXT NOT NULL,                                   -- Name of the branch (e.g., "Downtown Arena")
  capacity             BIGINT NOT NULL,                                 -- Maximum person capacity for this venue
  opening              BOOLEAN NOT NULL,                                -- Operational status: true if currently open for business
  
  -- Operations
  opening_hours_start  TIME WITHOUT TIME ZONE NOT NULL,                 -- Daily opening time (local time implies no date component)
  opening_hours_end    TIME WITHOUT TIME ZONE NOT NULL,                 -- Daily closing time (local time)

  -- Details & Content
  location_info        TEXT NOT NULL,                                   -- Address and contact details
  rule                 TEXT NOT NULL,                                   -- Specific usage rules or terms for this location
  facility             TEXT NOT NULL,                                   -- List or description of available facilities (amenities)
  description          TEXT NOT NULL,                                   -- Marketing description or "About Us" for the location

  -- Geography
  longitude            NUMERIC NOT NULL CHECK (
                          longitude >= '-180.0'::NUMERIC
                          AND longitude <= 180.0
                        ),                                              -- Geographic longitude with validation (-180 to 180)
  latitude             NUMERIC NOT NULL CHECK (
                          latitude >= '-90.0'::NUMERIC
                          AND latitude <= 90.0
                        ),                                              -- Geographic latitude with validation (-90 to 90)

  -- Meta / Audit
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the location was added

  -- Constraint: Creates a composite unique key to allow other tables (like location_managers) 
  -- to reference both ID and OrgID simultaneously, ensuring strict hierarchy ownership.
  UNIQUE (id, organization_id),

  -- Constraint: Ensures the location is linked to a valid organization.
  CONSTRAINT locations_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id)
);

-- =========================================================
-- Table: resource_types
-- Purpose: Types of resources for an organization
--          (e.g. badminton court, tennis court, meeting room).
-- =========================================================
CREATE TABLE IF NOT EXISTS public.resource_types (
  -- Identity
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the resource category

  -- Relationships
  organization_id UUID NOT NULL,                                   -- The organization that defines this resource type

  -- Content
  name            TEXT NOT NULL,                                   -- Name of the type (e.g., "Badminton Court")
  description     TEXT NOT NULL DEFAULT '',                        -- Optional details describing this type of resource

  -- Meta / Audit
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when this type was defined

  -- Constraint: Links the resource type to a specific organization.
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
  -- Identity
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the specific bookable item

  -- Relationships
  resource_type_id  UUID NOT NULL,                                   -- Classification of the resource (refers to resource_types)
  location_id       UUID NOT NULL,                                   -- Physical location where this resource exists

  -- Content
  name              TEXT NOT NULL,                                   -- Name/Number of the resource (e.g., "Court 1")

  -- Meta / Audit
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the resource was added

  -- Constraint: Ensures valid categorization of the resource.
  CONSTRAINT resources_resource_type_id_fkey
    FOREIGN KEY (resource_type_id) REFERENCES public.resource_types(id),
  
  -- Constraint: Ensures the resource is assigned to a valid physical location.
  CONSTRAINT resources_location_id_fkey
    FOREIGN KEY (location_id) REFERENCES public.locations(id)
);

-- =========================================================
-- Table: bookings
-- Purpose: Single booking for one resource (court/room) during
--          a continuous time range, made by a user.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.bookings (
  -- Identity
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),      -- Unique identifier for the reservation

  -- Relationships
  resource_id UUID NOT NULL,                                   -- The specific resource (court/room) being booked
  user_id     UUID NOT NULL,                                   -- The user who owns the reservation

  -- Schedule
  start_time  TIMESTAMPTZ NOT NULL,                            -- Start of the reservation period (UTC recommended)
  end_time    TIMESTAMPTZ NOT NULL,                            -- End of the reservation period (UTC recommended)

  -- Status
  status      booking_status NOT NULL DEFAULT 'pending',       -- Current state (pending, confirmed, cancelled)

  -- Meta / Audit
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the booking was made
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),              -- Timestamp when the booking was last modified

  -- Constraint: Links booking to a specific resource.
  CONSTRAINT bookings_resource_id_fkey
    FOREIGN KEY (resource_id) REFERENCES public.resources(id),
  
  -- Constraint: Links booking to a valid user account.
  CONSTRAINT bookings_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id),
  
  -- Constraint: Logic check to ensure the end time is strictly after the start time.
  CONSTRAINT bookings_time_range_valid
    CHECK (end_time > start_time)
);

-- =========================================================
-- Table: organization_members
-- Purpose: Members of an organization.
--          Must be a member to be a manager.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.organization_members (
  -- Relationships (Composite Key)
  organization_id UUID NOT NULL,                               -- The organization the user belongs to
  user_id         UUID NOT NULL,                               -- The user being added as a member

  -- Meta / Audit
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),          -- Timestamp when the user joined the organization
  
  -- Composite Primary Key: A user can only be a member of a specific organization once.
  PRIMARY KEY (organization_id, user_id),
  
  -- Constraint: Foreign key to organization. Cascade delete removes members if org is deleted.
  CONSTRAINT organization_members_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE,
  
  -- Constraint: Foreign key to users. Cascade delete removes membership if user is deleted.
  CONSTRAINT organization_members_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

-- =========================================================
-- Table: organization_managers
-- Purpose: Managers of an organization.
--          The owner is defined in the organizations table.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.organization_managers (
  -- Relationships (Composite Key)
  organization_id UUID NOT NULL,                               -- The organization ID
  user_id         UUID NOT NULL,                               -- The user ID designated as a manager
  
  -- Composite Primary Key: A user can only hold the manager role once per org.
  PRIMARY KEY (organization_id, user_id),
  
  -- Constraint: Enforces Hierarchy.
  -- Instead of referencing users/orgs directly, this references 'organization_members'.
  -- This ensures a user MUST be a generic 'member' before they can be promoted to 'manager'.
  CONSTRAINT organization_managers_member_fkey
    FOREIGN KEY (organization_id, user_id)
    REFERENCES public.organization_members(organization_id, user_id)
    ON DELETE CASCADE
);

-- =========================================================
-- Table: location_managers
-- Purpose: Grants manager permission to specific locations.
-- =========================================================
CREATE TABLE IF NOT EXISTS public.location_managers (
  -- Relationships (Composite Key)
  location_id     UUID NOT NULL,                               -- The specific location to be managed
  organization_id UUID NOT NULL,                               -- The organization owning that location
  user_id         UUID NOT NULL,                               -- The user designated as location manager
  
  -- Composite Primary Key: A user manages a specific location only once.
  PRIMARY KEY (location_id, user_id),
  
  -- Constraint: Enforces Role Prerequisite.
  -- References 'organization_members' to ensure the user is part of the organization first.
  CONSTRAINT location_managers_member_fkey
    FOREIGN KEY (organization_id, user_id)
    REFERENCES public.organization_members(organization_id, user_id)
    ON DELETE CASCADE,
  
  -- Constraint: Enforces Data Integrity / Tenancy.
  -- References 'locations' using (id, organization_id).
  -- This ensures that the 'location_id' strictly belongs to the 'organization_id' listed in this row.
  -- It prevents a user from Org A being assigned to manage a Location belonging to Org B.
  CONSTRAINT location_managers_location_org_fkey
    FOREIGN KEY (location_id, organization_id)
    REFERENCES public.locations(id, organization_id)
    ON DELETE CASCADE
);

-- =========================================================
-- Indexes
-- =========================================================

-- Index: Optimizes lookups for all locations belonging to a specific organization.
CREATE INDEX IF NOT EXISTS idx_locations_org
  ON public.locations (organization_id);

-- Index: Optimizes lookups for resource types within an organization.
CREATE INDEX IF NOT EXISTS idx_resource_types_org
  ON public.resource_types (organization_id);

-- Index: Optimizes filtering resources by their type (e.g. "Find all Badminton Courts").
CREATE INDEX IF NOT EXISTS idx_resources_type
  ON public.resources (resource_type_id);

-- Index: Optimizes lookups for all resources in a specific physical location.
CREATE INDEX IF NOT EXISTS idx_resources_location
  ON public.resources (location_id);

-- Index: Critical for performance. Used to find bookings for a specific resource 
-- within a time range (collision detection).
CREATE INDEX IF NOT EXISTS idx_bookings_resource_time
  ON public.bookings (resource_id, start_time, end_time);

-- Index: Optimizes retrieving a user's booking history or upcoming schedule.
CREATE INDEX IF NOT EXISTS idx_bookings_user_time
  ON public.bookings (user_id, start_time);