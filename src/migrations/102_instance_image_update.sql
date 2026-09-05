-- Per-instance digest snapshots make user-initiated version updates survive
-- later restart tasks without mutating the order's original purchase record.
ALTER TABLE xcloud_instances ADD COLUMN image_digest VARCHAR(255) NULL;
