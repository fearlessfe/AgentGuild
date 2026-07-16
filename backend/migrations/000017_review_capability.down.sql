ALTER TABLE reviews DROP CONSTRAINT IF EXISTS reviews_capability_nonempty;
ALTER TABLE reviews DROP COLUMN IF EXISTS capability;
