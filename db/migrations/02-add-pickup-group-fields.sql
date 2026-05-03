-- Migration: Add enable and location_id to pickup_groups, remove location.
-- Date: 2026-05-04

ALTER TABLE public.pickup_groups ADD COLUMN enable BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE public.pickup_groups ADD COLUMN location_id UUID NOT NULL REFERENCES public.locations(id);
ALTER TABLE public.pickup_groups DROP COLUMN location;
