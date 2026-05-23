CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

CREATE TABLE users (
    id         serial       PRIMARY KEY,
    email      text         NOT NULL UNIQUE,
    name       text,
    bio        text,
    avatar_url text,
    phone      varchar(11)  UNIQUE,
    prefecture varchar(20),
    age        int,
    is_active  boolean      NOT NULL DEFAULT true,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now()
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

CREATE TABLE products (
    id         serial      PRIMARY KEY,
    name       text        NOT NULL,
    image_url  text,
    cost_yen   int         NOT NULL,
    margin_yen int         NOT NULL,
    price_yen  int         GENERATED ALWAYS AS (cost_yen + margin_yen) STORED,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE memberships (
    id        serial      PRIMARY KEY,
    user_id   int         NOT NULL REFERENCES users(id),
    role      varchar(20) NOT NULL,
    joined_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, role)
);

CREATE TABLE categories (
    id        uuid PRIMARY KEY,
    parent_id uuid REFERENCES categories(id),
    name      text NOT NULL
);
