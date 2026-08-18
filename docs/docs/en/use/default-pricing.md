# Default and Suggested Pricing

The platform's default rates are **reference values**, not a market commitment. Administrators can adjust them at any time based on actual cluster costs, cloud provider bills, and operating expenses: resource rates in the **Global Settings → Billing** price table, and AI model token prices per model in **Global Settings → AI Assistant**. Changes only affect future usage.

## Conversion

- Settlement uses Credits. Reference conversion: **1 USD = 100 Credits**.
- When the billing display currency is USD and "Credits per fiat unit" is 100, the converted amounts in the billing overview match the USD reference prices below. For other currencies, adjust the ratio to the actual exchange rate.
- Suggested AI model price = the vendor's official list price (USD per 1M tokens) × 100.

## Platform resource default rates

The built-in defaults are rounded values derived from on-demand list prices of major public clouds (AWS, Alibaba Cloud, Tencent Cloud, GCP, etc.), roughly matching the cost of a mid-range on-demand instance:

| Meter | Default rate (Credits) | Unit | Reference basis |
| --- | --- | --- | --- |
| Build CPU | 10 | vCPU·minute | On-demand vCPU ≈ $0.03–0.05/hour |
| Build memory | 2 | GiB·minute | On-demand memory ≈ $0.004–0.007/GiB·hour |
| Runtime CPU | 30 | vCPU·hour | Same, settled per hourly observation window |
| Runtime memory | 6 | GiB·hour | Same |
| Persistent storage | 1 | GiB·day | Cloud disks ≈ $0.10–0.17/GiB·month |
| Gateway egress | 1 | GiB | Internet egress ≈ $0.08–0.12/GiB |
| Volume transfer | 0 (disabled) | GiB | Free by default; enable as needed |
| Gateway requests | 0 (disabled) | 1000 requests | Free by default; enable as needed |

With self-hosted clusters or committed-use discounts you can usually lower compute rates; raise them if you want to cover operating costs.

## AI model suggested prices

When you enter a provider model name in the model catalog, the form shows a suggested price converted from the official list price if the model is known to the platform, and can fill it in with one click. Unlisted models show no suggestion; price them from your provider's bill.

Official list prices (USD per 1M tokens) of the main listed models and the converted suggested Credits:

| Model | Input | Output | Cached input | Suggested input Credits | Suggested output Credits |
| --- | --- | --- | --- | --- | --- |
| OpenAI GPT-5.2 | $1.75 | $14.00 | $0.175 | 175 | 1400 |
| OpenAI GPT-5.1 / GPT-5 | $1.25 | $10.00 | $0.125 | 125 | 1000 |
| OpenAI GPT-5 mini | $0.25 | $2.00 | $0.025 | 25 | 200 |
| OpenAI GPT-4.1 | $2.00 | $8.00 | $0.50 | 200 | 800 |
| OpenAI GPT-4o | $2.50 | $10.00 | $1.25 | 250 | 1000 |
| OpenAI GPT-4o mini | $0.15 | $0.60 | $0.075 | 15 | 60 |
| OpenAI o3 | $2.00 | $8.00 | $0.50 | 200 | 800 |
| OpenAI o4-mini | $1.10 | $4.40 | $0.275 | 110 | 440 |
| Anthropic Claude Opus 4.5/4.6 | $5.00 | $25.00 | $0.50 | 500 | 2500 |
| Anthropic Claude Sonnet 4.x | $3.00 | $15.00 | $0.30 | 300 | 1500 |
| Anthropic Claude Haiku 4.5 | $1.00 | $5.00 | $0.10 | 100 | 500 |
| Google Gemini 3 Pro | $2.00 | $12.00 | $0.20 | 200 | 1200 |
| Google Gemini 3 Flash | $0.50 | $3.00 | $0.05 | 50 | 300 |
| Google Gemini 2.5 Pro | $1.25 | $10.00 | $0.125 | 125 | 1000 |
| Google Gemini 2.5 Flash | $0.30 | $2.50 | $0.03 | 30 | 250 |
| DeepSeek Chat (V3.2) | $0.28 | $0.42 | $0.028 | 28 | 42 |
| DeepSeek Reasoner | $0.55 | $2.19 | $0.055 | 55 | 219 |
| Alibaba Qwen3 Max | $1.20 | $6.00 | — | 120 | 600 |
| Alibaba Qwen Plus | $0.80 | $2.00 | — | 80 | 200 |
| Moonshot Kimi K2.5 | $4.00 | $20.00 | $0.70 | 400 | 2000 |
| Moonshot Kimi K2 | $4.00 | $16.00 | $1.00 | 400 | 1600 |
| ByteDance Doubao Seed 1.6 | $0.80 | $8.00 | — | 80 | 800 |
| Zhipu GLM-5 | $6.00 | $22.00 | — | 600 | 2200 |
| Zhipu GLM-4.7 | $4.00 | $18.00 | — | 400 | 1800 |
| Zhipu GLM-4.6 / 4.5 | $2.00 | $6.00 | — | 200 | 600 |

Notes:

- For **cached input**, Anthropic shows the cache-read price (10% of input), while DeepSeek and Kimi publish dedicated cache-hit prices. Vendors marked `—` publish no separate cached rate; set the cached prices to `0` so cached tokens are not billed separately.
- Some vendors use context tiers, batch discounts, or volume tiers; the table uses the standard tier. Dated or versioned variants of a listed model family reuse the family's suggested price.
- Suggested prices do not update automatically when a vendor changes pricing. Confirm against your provider's bill and adjust the model catalog manually; historical Runs keep their creation-time price snapshot and are unaffected.

## Sources

- OpenAI: `https://openai.com/api/pricing/`
- Anthropic: `https://claude.com/pricing`
- Google Gemini: `https://ai.google.dev/gemini-api/docs/pricing`
- DeepSeek: `https://api-docs.deepseek.com/quick_start/pricing`
- Alibaba Cloud Model Studio: `https://help.aliyun.com/zh/model-studio/models`
- Moonshot Kimi: `https://platform.moonshot.ai/docs/pricing/chat`
- Volcano Engine (Doubao): `https://www.volcengine.com/docs/82379/1544106`
- Zhipu BigModel: `https://docs.bigmodel.cn/cn/guide/models`
- AWS EC2 / EBS on-demand pricing: `https://aws.amazon.com/ec2/pricing/on-demand/`
