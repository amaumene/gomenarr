CREATE TABLE IF NOT EXISTS media (
    trakt_id BIGINT PRIMARY KEY,
    imdb VARCHAR(20) NOT NULL,
    number BIGINT NOT NULL DEFAULT 0,
    season BIGINT NOT NULL DEFAULT 0,
    title VARCHAR(500) NOT NULL,
    year BIGINT NOT NULL,
    on_disk BOOLEAN NOT NULL DEFAULT FALSE,
    file VARCHAR(1000) DEFAULT '',
    download_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_media_imdb ON media(imdb);
CREATE INDEX idx_media_on_disk ON media(on_disk);
CREATE INDEX idx_media_season ON media(season);
