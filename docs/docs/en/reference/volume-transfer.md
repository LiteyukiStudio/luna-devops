# Volume Transfer Configuration

Listing, creating, mounting, and deleting volumes does not require object storage. Import and export require an S3-compatible private bucket configured for both API and Worker. Without it, transfer endpoints return `volume_transfer.store_unavailable`.

## Minimal configuration

API and Worker use the same object-store settings, and Worker must be able to create Transfer Jobs:

```bash
VOLUME_TRANSFER_STORE=s3
VOLUME_TRANSFER_S3_ENDPOINT=https://s3.example.com
VOLUME_TRANSFER_S3_REGION=us-east-1
VOLUME_TRANSFER_S3_BUCKET=luna-volume-transfers
VOLUME_TRANSFER_S3_ACCESS_KEY_ID=replace-me
VOLUME_TRANSFER_S3_SECRET_ACCESS_KEY=replace-me
VOLUME_TRANSFER_CALLBACK_BASE_URL=https://luna-api.example.com
VOLUME_TRANSFER_JOB_IMAGE=liteyukistudio/luna-worker:<same-version>
```

Inject credentials through deployment Secrets. Keep the bucket private; never distribute long-lived object-store credentials to browsers, project workloads, or Transfer Jobs. `VOLUME_TRANSFER_CALLBACK_BASE_URL` must be an HTTPS API address reachable from runtime clusters and must not be derived from a request Host.

## Optional settings

| Variable | Default | Purpose |
| --- | --- | --- |
| `VOLUME_TRANSFER_S3_PATH_STYLE` | `true` | Keep enabled for S3 services that require path-style bucket addressing. |
| `VOLUME_TRANSFER_OBJECT_TTL` | `24h` | Retention for completed exports and verified objects available for retry. |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | Per-transfer limit, configurable from `1Gi` to `5Ti`; size it for object storage, networking, and cluster capacity. |
| `VOLUME_TRANSFER_SPOOL_DIR` | Dedicated directory under the system temporary directory | Local staging used by the API while receiving a part; it must be an absolute writable path. |
| `VOLUME_TRANSFER_SPOOL_MAX_BYTES` | `2Gi` | Total bytes that one API process may stage concurrently; it must fit at least one server-selected part. |
| `VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES` | `1Gi` | Disk space that must remain available after accepting a part. |
| `VOLUME_TRANSFER_SPOOL_ORPHAN_AGE` | `24h` | Safe age for cleaning the API's own stale spool files during startup. |

The platform selects the part size from the transfer length: at least `64MiB`, rounded up in MiB steps so the upload never exceeds S3's 10,000 multipart parts. A `5TiB` transfer uses `525MiB` parts. Web, CLI, and Transfer Jobs read this server value and must not choose smaller parts. Provision each API replica with at least `VOLUME_TRANSFER_SPOOL_MAX_BYTES + VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES` of available temporary disk.

## Verify and troubleshoot

1. Confirm API and Worker use the same PostgreSQL, Redis, and object-store configuration.
2. Confirm the Worker image contains `/usr/local/bin/luna-volume-transfer` and matches `VOLUME_TRANSFER_JOB_IMAGE`.
3. From the runtime cluster, verify DNS resolution and HTTPS access to the callback address.
   For `volume_transfer.completion_missing`, also verify Transfer Job-to-API callback connectivity, TLS, and temporary callback authentication.
4. Verify that the bucket allows multipart upload, Range read, Head, and Delete, and review its server-side encryption policy.
5. Run a small import and export. Check the transfer terminal state, SHA-256, cleanup of temporary Job/Secret/NetworkPolicy resources, and the OTel parent-child trace.

`volume_transfer.spool_busy` means that the current API replica reached its staging concurrency budget and the upload can resume later. For `volume_transfer.spool_insufficient_storage` or `volume_transfer.spool_unavailable`, inspect the spool mount, permissions, capacity, and inodes.

Non-standard cluster DNS labels or a Pod Security policy that forbids root access to Block devices can prevent a Transfer Job from starting. Inspect cluster events and adjust the cluster-specific compatibility setting; do not weaken security for the whole project namespace.
