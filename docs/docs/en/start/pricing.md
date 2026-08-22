# Set Prices

Administrators set resource prices under **Global Settings → Billing** and per-model token prices under **Global Settings → AI Assistant → AI Model Catalog**. A change affects only future usage; historical records keep the price snapshot captured when usage occurred.

## Credit conversion

Billing is settled in Credits. Select the display currency and configure the Credits represented by one currency unit. For example:

```text
Currency: USD
1 USD = 100 Credits
```

This ratio controls display conversion only and does not follow exchange rates automatically.

## Platform resource prices

Configure the price table by meter:

| Resource | Unit |
| --- | --- |
| Build CPU | vCPU-minute |
| Build memory | GiB-minute |
| Runtime CPU | vCPU-hour |
| Runtime memory | GiB-hour |
| Persistent storage | GiB-day |
| Gateway egress | GiB |
| Volume transfer | GiB |
| Gateway requests | 1,000 requests |

Use actual cloud invoices, reserved-resource discounts, redundancy, and operations cost. Built-in values are initialization references, not market-price commitments.

## AI model prices

The model catalog stores prices per one million tokens for ordinary input, output, cached input, and cached output. Luna DevOps settles the Provider-reported usage against the initiating user's personal wallet; AI cost is never assigned to a project space.

The form may show suggested prices for known models. They are entry aids and do not update automatically when a Provider changes pricing. Verify the Provider's current price list and your account-specific billing rules before saving.

## Pre-launch checks

1. Use one small build, runtime window, and model call to verify each unit.
2. Check the display currency, Credit conversion, and all four token prices.
3. After a price change, verify that new usage uses the new value while historical billing remains unchanged.
4. Set an unused meter to `0` only after confirming how that choice appears to users.
