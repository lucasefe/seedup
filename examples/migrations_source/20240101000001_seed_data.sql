-- +goose Up
INSERT INTO users (name, email) VALUES
    ('Alice', 'alice@example.com'),
    ('Bob', 'bob@example.com'),
    ('Charlie', 'charlie@example.com');

INSERT INTO posts (user_id, title, body) VALUES
    (1, 'Hello World', 'This is my first post!'),
    (1, 'Second Post', 'Another post from Alice'),
    (2, 'Bobs Post', 'Hello from Bob');

-- +goose Down
DELETE FROM posts;
DELETE FROM users;
