-- +goose Up
CREATE TABLE posts (
    id UUID NOT NULL PRIMARY KEY,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    title TEXT,
    description TEXT,
    url TEXT NOT NULL UNIQUE,
    published_at TIMESTAMP,
    feed_id UUID NOT NULL,
    FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE posts;
