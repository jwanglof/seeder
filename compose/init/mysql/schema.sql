CREATE TABLE users (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    email      VARCHAR(255) NOT NULL UNIQUE,
    name       VARCHAR(255),
    bio        TEXT,
    avatar_url VARCHAR(255),
    phone      VARCHAR(11)  UNIQUE,
    prefecture VARCHAR(20),
    age        INT,
    is_active  TINYINT(1)   NOT NULL DEFAULT 1,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE orders (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    user_id     INT          NOT NULL,
    status      ENUM('pending', 'paid', 'shipped', 'cancelled') NOT NULL,
    amount      INT          NOT NULL,
    title       TEXT,
    description TEXT,
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE comments (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    user_id    INT       NOT NULL,
    order_id   INT,
    body       TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id)  REFERENCES users(id),
    FOREIGN KEY (order_id) REFERENCES orders(id)
);

CREATE TABLE products (
    id         INT AUTO_INCREMENT PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    image_url  VARCHAR(255),
    cost_yen   INT          NOT NULL,
    margin_yen INT          NOT NULL,
    price_yen  INT AS (cost_yen + margin_yen) STORED,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE memberships (
    id        INT AUTO_INCREMENT PRIMARY KEY,
    user_id   INT         NOT NULL,
    role      VARCHAR(20) NOT NULL,
    joined_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id),
    UNIQUE (user_id, role)
);

CREATE TABLE categories (
    id        CHAR(36)     PRIMARY KEY,
    parent_id CHAR(36),
    name      VARCHAR(100) NOT NULL,
    FOREIGN KEY (parent_id) REFERENCES categories(id)
);
