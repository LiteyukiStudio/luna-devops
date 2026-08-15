ALTER TABLE volume_transfers
    ADD COLUMN logical_bytes bigint NOT NULL DEFAULT 0,
    ADD COLUMN data_sha256 text NOT NULL DEFAULT '';

ALTER TABLE volume_transfers
    ADD CONSTRAINT chk_volume_transfers_logical_bytes
        CHECK (logical_bytes >= 0),
    ADD CONSTRAINT chk_volume_transfers_data_sha256
        CHECK (data_sha256 = '' OR data_sha256 ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT chk_volume_transfers_data_digest_pair
        CHECK ((logical_bytes = 0) = (data_sha256 = ''));

COMMENT ON COLUMN volume_transfers.logical_bytes IS
    'Server-observed uncompressed data bytes, zero when an archive format does not report a raw-data digest.';
COMMENT ON COLUMN volume_transfers.data_sha256 IS
    'Server-observed SHA-256 of uncompressed data and never accepted from a public import request.';
