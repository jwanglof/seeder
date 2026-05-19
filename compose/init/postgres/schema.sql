CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

CREATE TABLE users (
    id         serial      PRIMARY KEY,
    email      text        NOT NULL UNIQUE,
    name       text,
    bio        text,
    avatar_url text,
    age        int,
    is_active  boolean     NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id          serial       PRIMARY KEY,
    user_id     int          NOT NULL REFERENCES users(id),
    status      order_status NOT NULL,
    amount      int          NOT NULL,
    title       text,
    description text,
    created_at  timestamptz  NOT NULL DEFAULT now()
);

CREATE TABLE comments (
    id         serial      PRIMARY KEY,
    user_id    int         NOT NULL REFERENCES users(id),
    order_id   int         REFERENCES orders(id),
    body       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
