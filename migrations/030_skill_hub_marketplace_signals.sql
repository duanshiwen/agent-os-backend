-- Skill Hub marketplace signals: ratings, download counts, and recommendation ordering.

ALTER TABLE skills
    ADD COLUMN IF NOT EXISTS download_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rating_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rating_sum BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rating_average NUMERIC NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS skill_ratings (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating INTEGER NOT NULL,
    CONSTRAINT chk_skill_ratings_rating CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT uq_skill_ratings_skill_user UNIQUE (skill_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_skill_ratings_skill_id ON skill_ratings(skill_id);
CREATE INDEX IF NOT EXISTS idx_skill_ratings_user_id ON skill_ratings(user_id);
CREATE INDEX IF NOT EXISTS idx_skills_catalog_downloads ON skills(status, visibility, download_count DESC, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_skills_catalog_rating ON skills(status, visibility, rating_average DESC, rating_count DESC, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_skills_catalog_recent ON skills(status, visibility, updated_at DESC);
