# 导入与导出数据卷

数据卷导入和导出是可恢复的后台任务。Filesystem 卷使用 `tar.gz`，Block 卷使用 `raw.zst`；不要把 Block 镜像按目录归档导入。

## 导入外部归档

1. 在项目空间“数据卷”页选择“导入归档”。
2. 选择运行集群、容量、StorageClass、访问模式和卷模式，再选择本地文件。
3. 确认归档格式和 SHA-256（如已知），完成 MFA 验证后开始上传。
4. 上传中断时重新打开页面并选择同一文件，可以从服务端确认的 offset 继续。
5. 上传完成后等待 Transfer 和数据卷都进入成功/就绪终态，再挂载到应用。

平台会拒绝路径穿越、越界链接、特殊设备文件、超容量展开和校验和不一致的归档。上传完成且临时对象尚未过期时，失败任务可以显式重试而无需重新上传；完整 SHA-256 仍由异步 Transfer Job 流式复核，只有 Transfer 进入“成功”才表示导入完成。

分片大小由服务端按归档总大小选择，页面会自动使用并保存服务端 offset；无需手动调整。除最后一片外，客户端发送更小分片会被拒绝。

## 导出并断点下载

1. 打开数据卷详情，选择“导出”。
2. 选择一致性模式：
   - **自动**：未使用卷直接导出；使用中卷优先创建 CSI Snapshot。
   - **快照**：必须使用 CSI Snapshot。快照是 crash-consistent，不等同于应用一致性备份。
   - **在线读取**：允许读取使用中的 Filesystem，内容可能在导出期间变化，仅 Owner/Admin 可选。
3. 等待 Transfer 进入“成功”，核对大小、SHA-256 和过期时间。
4. 完成 MFA 后开始下载。支持 File System Access API 的浏览器可在页面内通过 HTTP Range 暂停和继续；其他浏览器会使用原生下载直接写盘，不会先把完整归档保存在页面内存中。

成功的 Block 导出会在详情中同时提供同名 `raw.zst.manifest.json` 校验清单。清单只包含固定版本、卷模式、格式、导出完成时间、未压缩逻辑字节数、`fileCount: 0`、未压缩原始数据 SHA-256 和一致性模式。把清单与 `raw.zst` 一起保存；恢复或跨平台传输前，可以解压后重新计算 SHA-256 并与 `dataSHA256` 比对。清单不适用于 Filesystem `tar.gz`。

“已排队”或“运行中”不表示备份已经完成。归档默认保留 24 小时；过期后需要重新导出。

## 常见失败

| 错误码 | 处理方式 |
| --- | --- |
| `volume.snapshot_required` | 使用中的卷需要快照；Filesystem 可由 Owner/Admin 明确选择在线读取，Block 只能使用快照或在未挂载时导出。 |
| `volume.snapshot_unsupported` | 当前集群或 StorageClass 不支持快照；改用未挂载时导出或迁移到支持的存储类。 |
| `volume_transfer.chunk_checksum_mismatch` | 当前上传分块与 `Upload-Checksum` 不一致；保持服务端 offset，重新发送该分块。 |
| `volume_transfer.checksum_mismatch` | 完整归档与提交的 SHA-256 不一致；重新选择原文件或重新导出。 |
| `volume_transfer.archive_unsafe` | 归档包含越界路径、链接或特殊文件；重新生成安全归档。 |
| `volume_transfer.capacity_exceeded` | 展开后的数据超过目标容量；创建更大的目标卷后重试。 |
| `volume_transfer.callback_unavailable` | Transfer Job 无法访问平台回调地址；请管理员检查集群网络和回调配置。 |
| `volume_transfer.callback_unauthorized` | 临时回调凭据无效或已过期；取消旧任务并重试。 |
| `volume_transfer.download_unauthorized` | 下载票据或 Range 会话无效；重新完成 MFA 并授权下载。 |
| `volume_transfer.format_unsupported` | 归档格式与卷模式不匹配；Filesystem 使用 `tar.gz`，Block 使用 `raw.zst`。 |
| `volume_transfer.job_failed` | 运行集群中的传输任务失败；查看稳定错误码和集群事件后重试。 |
| `volume_transfer.completion_missing` | 任务已结束，但平台未收到完成确认；请重试，重复出现请联系管理员检查 Transfer Job 到 API 的回调连接和认证。 |
| `volume_transfer.store_unavailable` | 临时对象存储不可用或未配置；请管理员检查 API、Worker 和 bucket 配置。 |
| `volume_transfer.spool_busy` | API 暂存并发已满；稍候后继续上传。 |
| `volume_transfer.spool_unavailable` | API 暂存目录不可用；请管理员检查目录挂载与权限。 |
| `volume_transfer.spool_insufficient_storage` | API 暂存磁盘余量不足；请管理员释放或扩容暂存盘。 |
| `volume_transfer.state_conflict` | 任务已进入其他阶段或终态；刷新详情后再选择重试或重新创建。 |
| `volume_transfer.expired` | 上传、归档或下载授权已过期；重新上传、导出或授权。 |

管理员配置与对象存储排障见[数据卷传输配置](../reference/volume-transfer.md)。
