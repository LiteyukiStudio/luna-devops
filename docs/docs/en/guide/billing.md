# Billing and Spend Analysis

The billing page shows project-space balances, spend summaries, spend analysis, and ledger entries.

The project-space selector at the top controls the whole page. When the scope changes, the balance cards, monthly category summary, spend analysis, ledger, and usage records are all recalculated from the same project-space set. With no selection, regular users see related project spaces; platform administrators see all project spaces.

Platform administrators can also configure a real-world currency unit and conversion ratio in site settings. The top summary cards show the converted amount after credits, for example `1,012.24 Credits (1.01 CNY)`. This is display-only conversion; the ledger still settles in credits.

## Spend analysis

Spend analysis groups settled usage by project space and application. It shows total spend plus build, runtime, storage, gateway, and other spend.

Historical usage that is not linked to an application is grouped as “Unassigned application”. Ledger entries remain the audit trail for every balance change, while spend analysis is the faster way to find which project or application is driving cost.
