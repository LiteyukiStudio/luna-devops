# Import and Export Volumes

Volume imports and exports are resumable background operations. Filesystem volumes use `tar.gz`; Block volumes use `raw.zst`. Do not import a Block image as a directory archive.

## Import an external archive

1. On the project **Volumes** page, select **Import archive**.
2. Choose the runtime cluster, capacity, StorageClass, access mode, and volume mode, then select the local file.
3. Confirm the archive format and SHA-256 when known, complete MFA step-up, and start the upload.
4. If the upload is interrupted, reopen the page and select the same file to continue from the server-confirmed offset.
5. Wait until both the transfer and volume reach their successful/ready terminal states before mounting the volume.

The platform rejects path traversal, escaping links, special device files, expansion beyond capacity, and checksum mismatches. If the upload is complete and its temporary object has not expired, retry explicitly without uploading it again. The asynchronous Transfer Job still streams and verifies the full SHA-256; the import is complete only after the transfer reaches Succeeded.

The server selects a part size from the archive length, and the page automatically uses it together with the authoritative server offset. There is nothing to tune; a smaller non-final part is rejected.

## Export and resume a download

1. Open the volume details and select **Export**.
2. Select a consistency mode:
   - **Automatic** exports an unused volume directly and prefers a CSI snapshot for an in-use volume.
   - **Snapshot** requires CSI VolumeSnapshot support. It is crash-consistent, not an application-consistent backup.
   - **Live read** can read an in-use Filesystem while data changes and is available only to Owners and Admins.
3. Wait for the transfer to become Succeeded, then check its size, SHA-256, and expiration.
4. Complete MFA and start the download. Browsers with the File System Access API can pause and resume in the page with HTTP Range. Other browsers use a native download that streams directly to disk instead of buffering the whole archive in page memory.

A successful Block export also offers a same-name `raw.zst.manifest.json` verification manifest in the details view. It contains only the fixed schema version, volume mode, format, export completion time, uncompressed logical byte count, `fileCount: 0`, the SHA-256 of the uncompressed raw data, and the consistency mode. Keep it beside the `raw.zst`; before restore or cross-platform transfer, decompress the archive, recompute SHA-256, and compare it with `dataSHA256`. Filesystem `tar.gz` exports do not have this separate manifest.

Queued or Running does not mean the backup is complete. Archives are retained for 24 hours by default; export again after expiration.

## Common failures

| Code | Action |
| --- | --- |
| `volume.snapshot_required` | The in-use volume needs a snapshot. Owners/Admins may choose live read only for Filesystem; Block requires a snapshot or an unmounted export. |
| `volume.snapshot_unsupported` | The cluster or StorageClass does not support snapshots. Export while unmounted or migrate to supported storage. |
| `volume_transfer.chunk_checksum_mismatch` | The current upload part does not match `Upload-Checksum`. Keep the authoritative server offset and resend that part. |
| `volume_transfer.checksum_mismatch` | The complete archive does not match the submitted SHA-256. Select the original file again or rerun the export. |
| `volume_transfer.archive_unsafe` | The archive contains an escaping path, link, or special file. Generate a safe archive. |
| `volume_transfer.capacity_exceeded` | Expanded content exceeds the target capacity. Create a larger volume and retry. |
| `volume_transfer.callback_unavailable` | The Transfer Job cannot reach the Luna API callback. Ask an administrator to check cluster egress and callback settings. |
| `volume_transfer.callback_unauthorized` | The temporary callback credential is invalid or expired. Cancel the old transfer and retry. |
| `volume_transfer.download_unauthorized` | The download ticket or Range session is invalid. Complete MFA and authorize the download again. |
| `volume_transfer.format_unsupported` | The archive format does not match the volume mode. Use `tar.gz` for Filesystem and `raw.zst` for Block. |
| `volume_transfer.job_failed` | The transfer job failed in the runtime cluster. Check the stable code and cluster events, then retry. |
| `volume_transfer.completion_missing` | The Job ended, but the platform did not receive completion confirmation. Retry; if it repeats, ask an administrator to check Transfer Job-to-API callback connectivity and authentication. |
| `volume_transfer.store_unavailable` | Temporary object storage is unavailable or unconfigured. Ask an administrator to check API, Worker, and bucket settings. |
| `volume_transfer.spool_busy` | API staging reached its concurrency budget. Wait briefly, then resume the upload. |
| `volume_transfer.spool_unavailable` | The API spool directory is unavailable. Ask an administrator to inspect its mount and permissions. |
| `volume_transfer.spool_insufficient_storage` | The API spool disk lacks the required free space. Ask an administrator to free or expand it. |
| `volume_transfer.state_conflict` | The task has moved to another phase or terminal state. Refresh its details before retrying or creating a new transfer. |
| `volume_transfer.expired` | The upload, archive, or download authorization expired. Upload, export, or authorize again. |

Administrators can use [Volume Transfer Configuration](../reference/volume-transfer.md) for object-store setup and troubleshooting.
