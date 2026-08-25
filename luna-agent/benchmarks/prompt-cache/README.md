# Agent Prompt Cache 确定性基准

本基准用于比较两个 checkout 的 Agent 请求稳定前缀。它运行真实的 `ModelRuntime`、`ContextCompiler` 和 OpenAI-compatible 请求体构造逻辑，但使用固定内存 fixture 与本地捕获 Provider，不访问模型服务、数据库或网络。

## 测量边界

- 指标是相邻请求 JSON 序列化结果的 UTF-8 最长公共前缀字节数。
- 估算 Token 固定使用 `ceil(commonPrefixBytes / 4)`，只适合 checkout 间的相对比较。
- 这不是 Provider 实际缓存读取量、命中率、计费量或官方 usage；判断真实效果仍需生产链路上报的 `cacheReadInputTokens`。
- JSON 只保存请求字节数、SHA-256、结构计数、比较指标和断言结果，不保存原始 Prompt 或请求正文。

固定场景包括：

1. 同一 Run 在工具结果加入前后的多步调用。
2. 前一 Turn 的规范化用户输入与页面上下文转入历史后的相邻 Turn。
3. 既有工具被触碰而改变 LRU 顺序，以及新增工具。
4. 权威上一轮 usage 触发上下文压缩的前后请求。

每个场景还包含功能不变量断言，覆盖消息事实、页面上下文、工具 Schema、会话归属、近期历史和结构化摘要。任一断言失败时 runner 会在写出结果后以非零状态退出。

每份结果还包含三类可审计来源标识：

- `sourceRevision`：runner 自动读取当前 checkout 的完整 Git `HEAD`，不接受 `--label` 冒充来源。
- `implementationDigest`：对参与请求编译、运行时、Provider 能力门禁、持久化和 OpenAI-compatible 序列化的固定源码清单计算 SHA-256；旧 checkout 尚不存在的新实现文件使用稳定的 `<missing>` 标记，因此同一 harness 可以回放优化前代码。
- `harnessDigest`：对本目录的 runner、比较器、报告生成器和说明文档计算 SHA-256。

结果读取采用严格一致性校验：场景、Step 与断言 ID 必须唯一；每个 transition 必须恰好对应相邻 Step；公共前缀不得超过两侧请求；Token 估算、复用比例、断言计数和所有 summary 汇总必须能从明细精确重算。缺失或篡改明细会使命令以非零状态退出。

## 在 baseline 与 optimized checkout 运行

两个 checkout 必须包含同一版本的 `benchmarks/prompt-cache` 基础设施。结果目录应放在仓库外，避免把一次性基准数据作为长期源码提交。

```bash
pnpm --dir luna-agent benchmark:prompt-cache -- \
  --label baseline \
  --output /tmp/agent-cache-benchmark/baseline.json

pnpm --dir luna-agent benchmark:prompt-cache -- \
  --label optimized \
  --output /tmp/agent-cache-benchmark/optimized.json
```

runner 会读取 Git `HEAD` 和上述固定源码清单，但不读取系统时间、随机数或网络。同一 checkout、相同 Node/pnpm 依赖和相同工作树源码应产生逐字节一致的 JSON；`--label` 只用于报告中的可读名称，不参与来源证明。

## 对比并生成报告

在任一包含基准基础设施的 checkout 中执行：

```bash
pnpm --dir luna-agent benchmark:prompt-cache:compare -- \
  --baseline /tmp/agent-cache-benchmark/baseline.json \
  --optimized /tmp/agent-cache-benchmark/optimized.json \
  --output /tmp/agent-cache-benchmark/comparison.json \
  --report /tmp/agent-cache-benchmark/report.html
```

也可以稍后把单份结果或对比结果填入报告生成器：

```bash
pnpm --dir luna-agent benchmark:prompt-cache:report -- \
  --input /tmp/agent-cache-benchmark/comparison.json \
  --output /tmp/agent-cache-benchmark/report.html
```

`--output -` 表示写入 stdout，`report --input -` 可从 stdin 读取 JSON。

compare 在生成结果前执行以下门禁：

1. baseline 与 optimized 的 `harnessDigest` 必须一致，确保测量工具完全相同。
2. 两侧 `implementationDigest` 必须不同，避免把同一实现仅换 `--label` 后伪装成优化。
3. 场景、Step、transition、断言 ID 及说明必须逐项一致，不允许静默忽略任一侧缺失的数据。
4. 两份输入都必须通过明细和 summary 的严格派生值校验。

HTML 报告是无脚本、无字体/图片/CDN 的单文件，包含系统 light/dark 主题、可见键盘焦点、跳到主要内容链接、表格 caption、窄屏横向滚动、`prefers-reduced-motion` 和 A4 打印样式。测试会校验主要语义色在浅色与深色表面均达到 WCAG AA 4.5:1 对比度。
