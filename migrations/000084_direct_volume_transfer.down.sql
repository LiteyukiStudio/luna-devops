-- Intentionally irreversible. Restoring object-store references or multipart
-- part rows would re-enable a retired transfer protocol without its data.
SELECT 1;
