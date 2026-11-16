DROP TABLE IF EXISTS public.coach_group_invitations;
DROP TABLE IF EXISTS public.coach_group_members;
DROP TABLE IF EXISTS public.coach_groups;
DROP TABLE IF EXISTS public.coach_certificates;
DROP TABLE IF EXISTS public.coach_sports;
DROP TABLE IF EXISTS public.coaches;
ALTER TABLE public.notifications DROP COLUMN IF EXISTS ref_id;
ALTER TABLE public.notifications DROP COLUMN IF EXISTS ref_type;
