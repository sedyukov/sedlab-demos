-- 0002: orders get an optional due date (shows how the schema evolves without downtime: nullable column + index).
ALTER TABLE orders ADD COLUMN due_date TEXT;
CREATE INDEX orders_ws_due ON orders (workspace_id, due_date);
