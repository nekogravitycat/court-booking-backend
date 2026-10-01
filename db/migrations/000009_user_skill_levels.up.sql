-- Each account may report one skill level per sport. Existing orders remain
-- historical snapshots; old enrollment data is not a profile declaration.
CREATE TABLE public.user_skill_levels (
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    sport_id UUID NOT NULL,
    skill_level INTEGER NOT NULL CHECK (skill_level BETWEEN 1 AND 100),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, sport_id),
    FOREIGN KEY (sport_id, skill_level)
        REFERENCES public.skill_levels(sport_id, level)
);
