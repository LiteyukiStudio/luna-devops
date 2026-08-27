# Runtime Data Persistence

Use project volumes for database files, uploads, and other data that must survive a release. `emptyDir` disappears with its Pod and is suitable only for cache or temporary files.

## Create or connect a volume

Open **Volumes** in the project space and choose a source:

- **Blank volume**: create a platform-managed PVC.
- **Existing PVC**: reference an existing PVC. Adopting it transfers lifecycle ownership to Luna DevOps and requires explicit confirmation.
- **VolumeSnapshot**: restore from an available snapshot in the same cluster.
- **Archive import**: use `tar.gz` for Filesystem volumes and `raw.zst` for Block volumes.

Select the cluster, capacity, StorageClass, access mode, and volume mode. A `WaitForFirstConsumer` StorageClass may remain Preparing until its first mount; the deployment can still select it.

## Mount it in an application

1. Edit a deployment in the same project space and cluster.
2. Select the project volume under **Volumes**.
3. Enter an absolute `mountPath` for Filesystem or an absolute `devicePath` for Block.
4. Save and create a new Release.

Deleting an application or deployment only unbinds the volume; it does not automatically delete the PVC. Capacity can only increase. Create a new volume and migrate the data to change StorageClass, access mode, or volume mode.

## Import and export

An import writes the archive directly into the target volume mounted by a temporary Transfer Pod. It does not pass through object storage, browser-side pre-hashing, or a complete local platform spool. The API and Transfer Pod independently calculate and compare the digest during that same stream. An interrupted connection cannot resume or retry against the same destination. Delete the failed import volume and start a fresh import so old partial data cannot mix with the new archive. Recovery is complete only when both the Transfer and volume reach succeeded and ready terminal states.

Export modes are:

- **Automatic**: export an idle volume directly; prefer a CSI Snapshot for an attached volume.
- **Snapshot**: require a CSI Snapshot. It is crash-consistent, not application-consistent.
- **Online read**: read an attached Filesystem volume while data may change; Owner/Admin only.

Export content streams directly from the Transfer Pod. Luna DevOps does not retain a server-side archive for repeated downloads, and downloads cannot resume. Authorization uses a short-lived, single-use ticket bound to the current user, session, and Transfer. A backup is complete only after the download finishes and the Transfer is Succeeded; start a new export after an interruption.

> Permanently deleting a managed volume deletes the underlying PVC and cannot be undone. Review mounts and transfers and complete any backup first.

## Administrator prerequisites

Import and export do not require S3 or another object store. The runtime cluster must allow Luna DevOps to create a temporary Transfer Pod, mount the PVC, and stream through Kubernetes exec from the API. The transfer helper image must match the Worker version. The reverse proxy must also allow long-lived request and response streams without buffering their bodies. Streams with no progress are reclaimed, and a single stream can run for at most seven days. See [API Configuration](/en/start/configuration/api#volume-import-and-export) and [Worker Configuration](/en/start/configuration/worker#volume-import-and-export).

## Platform history retention

**Global Settings → Data Retention** cleans expired events, notifications, task history, and logs. It **does not delete project volumes, billing ledgers, or running-task data**. Preview the matched records before irreversible manual cleanup, and export logs to an external system when long-term retention is required.
