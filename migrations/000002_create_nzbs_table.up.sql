CREATE TABLE IF NOT EXISTS nzbs (
    id SERIAL PRIMARY KEY,
    trakt_id BIGINT NOT NULL,
    link TEXT NOT NULL,
    length BIGINT NOT NULL,
    title TEXT NOT NULL,
    failed BOOLEAN NOT NULL DEFAULT FALSE,
    parsed_title VARCHAR(500),
    parsed_year BIGINT,
    parsed_season BIGINT,
    parsed_episode BIGINT,
    resolution VARCHAR(20),
    source VARCHAR(50),
    codec VARCHAR(20),
    validation_score INTEGER NOT NULL DEFAULT 0,
    quality_score INTEGER NOT NULL DEFAULT 0,
    total_score INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_nzbs_trakt_id ON nzbs(trakt_id);
CREATE INDEX idx_nzbs_failed ON nzbs(failed);
CREATE INDEX idx_nzbs_validation_score ON nzbs(validation_score);
CREATE INDEX idx_nzbs_quality_score ON nzbs(quality_score);
CREATE INDEX idx_nzbs_total_score ON nzbs(total_score);
