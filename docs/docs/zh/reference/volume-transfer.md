# 数据卷传输配置

数据卷列表、创建、挂载和删除不依赖对象存储。只有启用导入/导出时，API 和 Worker 才需要配置 S3-compatible 私有 bucket；未配置时传输接口返回 `volume_transfer.store_unavailable`。

## 最小配置

API 与 Worker 使用相同的对象存储配置，Worker 还必须能够创建 Transfer Job：

```bash
VOLUME_TRANSFER_STORE=s3
VOLUME_TRANSFER_S3_ENDPOINT=https://s3.example.com
VOLUME_TRANSFER_S3_REGION=us-east-1
VOLUME_TRANSFER_S3_BUCKET=luna-volume-transfers
VOLUME_TRANSFER_S3_ACCESS_KEY_ID=replace-me
VOLUME_TRANSFER_S3_SECRET_ACCESS_KEY=replace-me
VOLUME_TRANSFER_CALLBACK_BASE_URL=https://luna-api.example.com
VOLUME_TRANSFER_JOB_IMAGE=liteyukistudio/luna-worker:<same-version>
```

凭据必须通过部署平台 Secret 注入。Bucket 必须保持私有；不要给浏览器、项目工作负载或 Transfer Job 分发对象存储长期凭据。`VOLUME_TRANSFER_CALLBACK_BASE_URL` 必须是运行集群可访问的 HTTPS API 地址，不能使用请求 Host 动态拼接。

## 可选配置

| 配置项 | 默认值 | 用途 |
| --- | --- | --- |
| `VOLUME_TRANSFER_S3_PATH_STYLE` | `true` | S3 服务需要 path-style bucket 地址时保留默认值。 |
| `VOLUME_TRANSFER_OBJECT_TTL` | `24h` | 成功导出和失败重试所需临时对象的保留时间。 |
| `VOLUME_TRANSFER_MAX_BYTES` | `100Gi` | 单次传输上限，可配置范围为 `1Gi`–`5Ti`；必须按对象存储、网络和集群容量评估。 |
| `VOLUME_TRANSFER_SPOOL_DIR` | 系统临时目录下的专用目录 | API 接收每个分片时使用的本地暂存目录；必须是可写的绝对路径。 |
| `VOLUME_TRANSFER_SPOOL_MAX_BYTES` | `2Gi` | 单个 API 进程允许同时暂存的总字节数；至少要容纳一个服务端选择的分片。 |
| `VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES` | `1Gi` | 接收分片后仍需保留的磁盘可用空间。 |
| `VOLUME_TRANSFER_SPOOL_ORPHAN_AGE` | `24h` | API 启动时清理自身旧暂存文件的安全年龄。 |

平台按传输大小选择分片：最小 `64MiB`，并按 MiB 向上取整，确保不超过 S3 的 10,000 个 multipart parts；`5TiB` 传输使用 `525MiB` 分片。Web、CLI 和 Transfer Job 会读取服务端返回值，不应自行指定更小分片。请为每个 API 副本准备至少 `VOLUME_TRANSFER_SPOOL_MAX_BYTES + VOLUME_TRANSFER_SPOOL_MIN_FREE_BYTES` 的可用临时磁盘。

## 验证与排障

1. 确认 API 与 Worker 使用同一 PostgreSQL、Redis 和对象存储配置。
2. 确认 Worker 镜像同时包含 `/usr/local/bin/luna-volume-transfer`，并与 `VOLUME_TRANSFER_JOB_IMAGE` 版本一致。
3. 从运行集群验证能够解析并通过 HTTPS 访问回调地址。
   若出现 `volume_transfer.completion_missing`，同时检查 Transfer Job 到 API 回调地址的网络连通性、TLS 和临时回调认证。
4. 验证 bucket 允许 multipart upload、Range read、Head 和 Delete，并检查服务端加密策略。
5. 发起一个小型导入和导出，确认 Transfer 终态、SHA-256、临时 Job/Secret/NetworkPolicy 清理以及 OTel 父子链路。

`volume_transfer.spool_busy` 表示当前 API 副本已达到暂存并发预算，稍后继续即可；`volume_transfer.spool_insufficient_storage` 或 `volume_transfer.spool_unavailable` 需要检查暂存目录挂载、权限、容量和 inode。

非标准集群 DNS 标签或限制 Block 设备 root 访问的 Pod Security 策略可能阻止 Transfer Job。此时先查看集群事件并调整集群侧兼容配置，不要放宽整个项目空间的安全策略。
