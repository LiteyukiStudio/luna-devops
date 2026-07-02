# Billing and Spend Analysis

The billing page shows user balance, spend summaries, spend analysis, and ledger entries.

Project spaces are the source of spend. Charges are deducted from the current billing owner's user wallet. If ownership is transferred later, new charges go to the new owner; old ledger entries stay with the original billed user.

For legacy releases without a deployment target reference, the platform backfills or bills them only when the matching deployment target is unambiguous.

Legacy deployment targets without a delete status are normalized as active when they are not deleted.

The top balance card is a user-account summary. Period spend, today spend, and pending spend follow the current project-space scope. The top page toolbar lets users choose both a billing period and a project-space scope. Period presets include this week, last 7 days, this month, last 30 days, this year, and last year, and users can also pick a custom date range. Spend analysis, ledger entries, usage records, and period categories are filtered by the selected period. With no project-space selection, regular users see their own billed ledger; platform administrators see all billing data.

Platform administrators can also configure a real-world currency unit and conversion ratio in site settings. The top summary cards show the converted amount after credits, for example `1,012.24 Credits (1.01 CNY)`. This is display-only conversion; the ledger still settles in credits.

When platform administrators open user management, the user list shows each user's wallet balance for quick account checks. Users without a wallet record are displayed as 0 credits.

## Spend analysis

Spend analysis groups settled usage by project space, application, and deployment config. It shows total spend plus build, runtime, storage, gateway, and other spend.

CPU, memory, and storage are settled by deployment-config windows. Gateway traffic is settled by access-route windows and attributed back to the route's deployment config. Build usage is settled by BuildRun and attributed back to the build's deployment config.

Usage that is not linked to an application or deployment config is grouped as “Unassigned application” or “Unassigned deployment config”. Ledger entries remain the audit trail for every balance change, while spend analysis is the faster way to find which project, application, or deployment config is driving cost.

## Gateway traffic

Gateway charges are based on response egress traffic from platform-managed access routes. The platform does not read every Pod's external traffic directly, and cluster-internal service calls are not counted as public gateway traffic.

A gateway or external collector reports response bytes by GatewayRoute and time window. The platform converts the bytes to GiB with the `gateway.egress_gib` billing rule and writes settled usage records. Request count is currently stored for audit and future anti-abuse analysis; it is disabled for billing by default.
