# Manage Project Volumes

A project volume is persistent data owned independently from an application. It can be mounted by deployment targets in the same project and runtime cluster. Deleting an application or unbinding a target does not automatically delete the volume.

## Prerequisites

- Project Viewers can inspect volumes; Developers can create, reference, adopt, and expand them. Adopting an existing PVC requires explicit confirmation that the platform will own its lifecycle.
- Exporting requires project Owner/Admin `volume:export` permission. Permanently deleting or detaching an external reference requires `volume:delete`.
- The runtime cluster must be reachable. Creating a blank volume also requires a StorageClass offered by that cluster.

## Create a volume

1. Open a project and select **Volumes**.
2. Select **Create volume**, then enter the name, runtime cluster, capacity, StorageClass, access mode, and volume mode.
3. Choose a source:
   - **Blank volume** creates a new platform-managed PVC.
   - **Existing PVC** references or adopts a PVC in the project namespace. A reference is not deleted or billed as platform storage; adoption transfers lifecycle ownership to the platform and requires confirmation.
   - **VolumeSnapshot** restores an available snapshot from the same cluster.
4. After submission, the volume normally becomes Ready. A `WaitForFirstConsumer` StorageClass may keep it Provisioning/Pending until the first mount; it is already selectable at that point, and the Pod becomes the first consumer that triggers binding.

The AI assistant can list project volumes and StorageClasses for the target cluster, then create a blank volume, reference an existing PVC, or restore from a VolumeSnapshot after the parameters are confirmed. Before blank creation or snapshot restore, it must read the target cluster's StorageClasses and submit a returned name together with capacity, access mode, and volume mode; creation is not attempted when any value is missing. High-risk actions such as adopting an existing PVC require approval bound to the current parameters. The assistant never substitutes a placeholder ID or `emptyDir` for required persistent storage.

When the cluster is unreachable, the page reports the observation as unavailable instead of displaying stale PVC state. Restore connectivity, then refresh or retry the last operation.

## Capacity quota and balance

A managed volume reserves its full capacity when creation, adoption, snapshot restore, or import begins. An expansion reserves only the increase. An externally referenced volume does not consume managed-capacity quota and is not charged as platform-managed storage. The project billing owner must have a positive balance before starting or retrying managed-volume creation, import, or expansion.

Administrators set the managed-volume limit for each project under **Global Settings → Billing**. The default `0` means unlimited. Lowering the limit never truncates or deletes an existing volume. If existing usage is already above the new limit, creation, expansion, and their retries are rejected until an administrator raises the limit or the project deletes unused managed volumes.

A failed or cancelled initial creation/import releases that reservation. A failed expansion releases only its increase. Capacity is released after permanent managed-volume deletion completes.

## Mount it in a deployment target

1. Edit an application deployment target and add a mount under Data volumes. Persistent data can select either a Ready volume or a Provisioning volume waiting for its first consumer; use `emptyDir` only for temporary data.
2. Select **Project volume**, then search the available volumes in the same cluster.
3. Enter an absolute `mountPath` for a Filesystem volume or an absolute `devicePath` for a Block volume. Read-only and shared mounts are allowed only when the access mode permits them.
4. Save the target and create a new release.

The volume is Reserved first. It becomes In use only after the worker authoritatively observes the expected PVC on the Kubernetes workload. Switching or removing a volume also requires a successful release; the volume returns to Available only after the old workload no longer references it.

When an app marketplace template declares persistent storage, its install form requires an attachable project volume from the same project and cluster with a matching volume mode. A Provisioning volume waiting for its first consumer is selectable, and the template Pod triggers PVC binding. Installation is rejected without a real selected volume; the platform neither creates an implicit PVC nor falls back to `emptyDir`.

## Expand or delete

- Capacity can only increase. To change StorageClass, access mode, or volume mode, create another volume and migrate the data.
- Review the deletion impact first. Reserved or active mounts and running transfers block deletion.
- Permanently deleting a managed volume deletes its PVC and cannot be undone. An externally referenced volume can only be detached; its PVC remains. Both actions require Owner/Admin permission and explicit confirmation.

For migration and backup, see [Import and Export Volumes](./volume-transfer.md).
