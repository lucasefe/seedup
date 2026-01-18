-- +goose Up
-- +goose StatementBegin
-- Sequences
CREATE SEQUENCE "public"."goose_db_version_id_seq" START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 CACHE 1;
CREATE SEQUENCE "public"."posts_id_seq" START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 CACHE 1;
CREATE SEQUENCE "public"."users_id_seq" START WITH 1 INCREMENT BY 1 MINVALUE 1 MAXVALUE 2147483647 CACHE 1;

-- Tables
CREATE TABLE "public"."posts" (
    "id" integer NOT NULL DEFAULT nextval('posts_id_seq'::regclass),
    "user_id" integer NOT NULL,
    "title" text NOT NULL,
    "body" text,
    "created_at" timestamp without time zone DEFAULT now()
);
CREATE TABLE "public"."users" (
    "id" integer NOT NULL DEFAULT nextval('users_id_seq'::regclass),
    "name" text NOT NULL,
    "email" text NOT NULL,
    "created_at" timestamp without time zone DEFAULT now()
);

-- Primary keys
ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_pkey" PRIMARY KEY (id);
ALTER TABLE "public"."users" ADD CONSTRAINT "users_pkey" PRIMARY KEY (id);

-- Unique constraints
ALTER TABLE "public"."users" ADD CONSTRAINT "users_email_key" UNIQUE (email);

-- Foreign keys
ALTER TABLE "public"."posts" ADD CONSTRAINT "posts_user_id_fkey" FOREIGN KEY (user_id) REFERENCES users(id);

-- +goose StatementEnd
