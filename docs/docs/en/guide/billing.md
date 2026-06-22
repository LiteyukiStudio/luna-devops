# Billing and Spend Analysis

The billing page shows user balance, spend summaries, spend analysis, and ledger entries.

Project spaces are the source of spend. Charges are deducted from the current billing owner's user wallet. If ownership is transferred later, new charges go to the new owner; old ledger entries stay with the original billed user.

For legacy releases without a deployment target reference, the platform backfills or bills them only when the matching deployment target is unambiguous.

The top balance, today spend, month spend, and pending spend cards are user-account summaries. The project-space selector lives in the detail toolbar and filters the monthly categories, spend analysis, ledger, and usage records below it. With no selection, regular users see their own billed ledger; platform administrators see all billing data.

Platform administrators can also configure a real-world currency unit and conversion ratio in site settings. The top summary cards show the converted amount after credits, for example `1,012.24 Credits (1.01 CNY)`. This is display-only conversion; the ledger still settles in credits.

## Spend analysis

Spend analysis groups settled usage by project space and application. It shows total spend plus build, runtime, storage, gateway, and other spend.

Historical usage that is not linked to an application is grouped as “Unassigned application”. Ledger entries remain the audit trail for every balance change, while spend analysis is the faster way to find which project or application is driving cost.

## Gateway traffic

Gateway charges are based on response egress traffic from platform-managed access routes. The platform does not read every Pod's external traffic directly, and cluster-internal service calls are not counted as public gateway traffic.

A gateway or external collector reports response bytes by GatewayRoute and time window. The platform converts the bytes to GiB with the `gateway.egress_gib` billing rule and writes settled usage records. Request count is currently stored for audit and future anti-abuse analysis; it is disabled for billing by default.
