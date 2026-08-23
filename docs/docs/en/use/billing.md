# Billing and Cost Analysis

The billing page shows account balance, spending trends, and the projects, applications, and deployment targets that generated costs.

Project costs are charged to the project's **billing owner**. After ownership changes, new costs are charged to the new owner; historical charges are not transferred. Verify the project and requester before accepting a billing ownership transfer.

Users can only view their own bills. Administrators can view all users or a selected user. Filter by time, user, and project. Use billing transactions for auditing and cost analysis to find the largest sources of spending.

Each ledger entry shows both its standard reason and the specific description recorded when it was created. Empty descriptions and descriptions that duplicate the reason are omitted.

Settlement uses Credits. Administrators may configure a display currency and conversion ratio, but this does not change settlement or recalculate historical bills.

## Price table

The **Price table** tab lists platform resource meters, including build CPU and memory, runtime resources, persistent storage, and gateway traffic. AI Token prices are managed per model in **Global Settings → AI Assistant**, rather than in the generic price table. See [Set Prices](/en/start/pricing) for setup.

Disabled resource meters remain visible but do not currently generate charges. Resource price and state changes only affect future usage; settled bills are not recalculated.

## Volume charges

Platform-managed volumes continue to produce storage usage while they are Available, Reserved, or In use. Metering prefers the PVC capacity observed live from Kubernetes and falls back to the recorded requested capacity only while the cluster is unavailable. Externally referenced volumes do not produce managed-storage charges.

Actual import and export bytes use the `storage.transfer_gib` meter. Each succeeded, failed, cancelled, or expired Transfer job is settled at most once. A retry creates a new Transfer and is settled independently from the bytes it actually moves. This meter is disabled with a price of `0` by default; administrators can enable it and set its price in the **Global Settings → Billing** price table.

## Cost analysis

Cost analysis groups settled usage by project, application, and deployment target and separates build, runtime, storage, gateway, and other costs.

Usage without a specific application or deployment target appears as **Unassigned**. For unexpected costs, narrow the time range and inspect each project, application, and deployment target.

Runtime resources are observed on natural-minute boundaries through Kubernetes `metrics.k8s.io`, while billing still creates one CPU and one memory record for each completed natural hour. Each minute interval is calculated as `max(effective request × desired replicas, actual usage)`, multiplied by that interval's effective duration, and then aggregated into vCPU-hours and GiB-hours. A workload created partway through a minute is prorated, and overlapping intervals are not counted twice.

If `metrics.k8s.io` is unavailable but Kubernetes confirms that the workload is running, that minute uses only the effective request floor. When the corresponding request policy is `0%`, the platform does not invent usage for that dimension. Entirely missing minutes are neither interpolated nor extrapolated, and minutes with zero desired replicas generate no runtime usage. The coverage ratio in billing metadata is the observed running duration divided by the full hour. Historical observations retain their effective request and cluster policy snapshot, and settled bills are never recalculated after a policy change.

## AI tokens

AI usage is billed only from schema-valid prompt, completion, cached-prompt, and cache-write prompt usage in the Provider's official `usage` object and is charged to the user who started the conversation. Completion usage already includes reasoning, so reasoning details are not charged again. Three prices per million tokens are snapshotted when the Run is created; catalog changes affect only later Runs. If Provider usage is missing or invalid, the call enters reconciliation and is not settled from zeroes or the credit hold.

## Gateway traffic

Gateway charges use response egress traffic from platform-managed routes. Traffic between services inside the cluster is not included.

Traffic collection is optional. An administrator installs `Luna Gateway Traffic Probe` for each target runtime cluster from the App Marketplace. For the reporting mechanism, authentication, configuration options, and troubleshooting, see [Gateway Traffic Probe](../reference/gateway-traffic-probe.md).

The billing page may show:

- **Not installed**: the cluster has no collector.
- **Waiting for reports**: the collector is ready but no valid traffic data has arrived.
- **Unavailable**: the platform cannot currently read the cluster state.

Already settled usage remains in billing history.
