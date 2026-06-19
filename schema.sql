CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    price REAL NOT NULL,
    description TEXT,
    image_url TEXT
);

CREATE TABLE IF NOT EXISTS orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    telegram_user_id INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(product_id) REFERENCES products(id)
);

INSERT OR IGNORE INTO products (id, name, price, description, image_url) VALUES 
(1, 'Go Gopher Plushie', 250000, 'Boneka maskot Go original edisi terbatas.', 'https://placehold.co/150'),
(2, 'Vue 3 Premium Mug', 120000, 'Mug keramik berkualitas tinggi dengan logo Vue.', 'https://placehold.co/150');
