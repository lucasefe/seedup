-- Seed query file for the "dev" seed set
-- This file defines which data to extract from the source database
-- Each temp table is named: pg_temp."seed.<schema>.<table>"

-- Select all users (in a real scenario, you'd limit this)
INSERT INTO pg_temp."seed.public.users" (id, name, email, created_at)
SELECT id, name, email, created_at
FROM public.users;

-- Select posts for those users
INSERT INTO pg_temp."seed.public.posts" (id, user_id, title, body, created_at)
SELECT p.id, p.user_id, p.title, p.body, p.created_at
FROM public.posts p
WHERE p.user_id IN (SELECT id FROM pg_temp."seed.public.users");
