# Billing and Cost Analysis

The billing page shows account balance, spending trends, and the projects, applications, and deployment targets that generated costs.

Project costs are charged to the project's **billing owner**. After ownership changes, new costs are charged to the new owner; historical charges are not transferred. Verify the project and requester before accepting a billing ownership transfer.

Users can only view their own bills. Administrators can view all users or a selected user. Filter by time, user, and project. Use billing transactions for auditing and cost analysis to find the largest sources of spending.

When an administrator selects **All users**, the balance card shows **Platform wallet total**: the sum of every user wallet. It is not an account balance, so no balance status or low-balance warning is shown. Select a specific user to see that account's balance status.

## Time Range Rules

- Costs are assigned to a time range by **when the usage occurred**: each usage record counts entirely in the range that contains the start of its metering period. Settlement time and period overlap are not used. For example, runtime usage for 23:00–24:00 settled at 00:05 the next day still counts toward the previous day.
- Ledger entries without a usage record, such as manual recharges and adjustments, use the time they were posted.
- **Today** and **this month** follow the calendar day and month in your browser's time zone, and the selected start and end dates are interpreted in the same local time zone.
- The **Occurred at** column in ledger entries and usage records shows the time used for filtering and sorting.

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

Volume storage and volume import/export transfers belong to the project, not to any deployment target, and appear as one **Project storage** row in cost analysis. Other usage without a specific application or deployment target appears as **Unassigned**. For unexpected costs, narrow the time range and inspect each project, application, and deployment target.

Runtime resources are observed on natural-minute boundaries through Kubernetes `metrics.k8s.io`, while billing still creates one CPU and one memory record for each completed natural hour. Each minute interval is calculated as `max(effective request × desired replicas, actual usage)`, multiplied by that interval's effective duration, and then aggregated into vCPU-hours and GiB-hours. A workload created partway through a minute is prorated, and overlapping intervals are not counted twice.

The effective request is the per-replica requests read from the workload Pod template at observation time, summed over every container (the application container and sidecars). Actual usage is likewise summed over every Pod and container of the workload, excluding deployment hook Job Pods. Changing the runtime cluster resource policy or a deployment's quota therefore changes the billing floor only after the workload is redeployed.

If `metrics.k8s.io` is unavailable but Kubernetes confirms that the workload is running, that minute uses only the effective request floor. When the Pod template has no request for a dimension (for example a `0%` policy), the platform does not invent usage for that dimension. Entirely missing minutes are neither interpolated nor extrapolated, and minutes with zero desired replicas generate no runtime usage. The coverage ratio in billing metadata is the observed running duration divided by the full hour. Historical observations retain their effective request and the cluster policy used to render the workload, and settled bills are never recalculated after a policy change.

The runtime cost estimate on the deployment editor is a range. The floor is the selected runtime cluster's request percentage × quota × replicas, and the ceiling is the limit percentage × quota × replicas. With autoscaling, the floor uses the minimum replicas and the ceiling uses the maximum replicas. If the policy sets a limit to `0%` (no limit), there is no cost ceiling. The estimate does not include sidecar resources.

## AI tokens

AI usage is billed only from schema-valid prompt, completion, cached-prompt, and cache-write prompt usage in the Provider's official `usage` object and is charged to the user who started the conversation. Completion usage already includes reasoning, so reasoning details are not charged again. Three prices per million tokens are snapshotted when the Run is created; catalog changes affect only later Runs. If Provider usage is missing or invalid, the call enters reconciliation and is not settled from zeroes or the credit hold.

## Gateway traffic

Gateway charges use response egress traffic from platform-managed access routes. Traffic between services inside the cluster is not included.

Traffic collection is optional. An administrator installs `Luna Gateway Traffic Probe` for each target runtime cluster from the App Marketplace. For the reporting mechanism, authentication, configuration options, and troubleshooting, see [Gateway Traffic Probe](../reference/gateway-traffic-probe.md).

The billing page may show:

- **Not installed**: the cluster has no collector.
- **Waiting for reports**: the collector is ready but no valid traffic data has arrived.
- **Unavailable**: the platform cannot currently read the cluster state.

Already settled usage remains in billing history.
