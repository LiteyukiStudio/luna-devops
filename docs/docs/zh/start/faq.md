# 常见问题

## 应该选择仓库 Dockerfile 还是平台模板？

- 项目已经维护 Dockerfile：选择“仓库 Dockerfile”。
- 项目没有 Dockerfile，或希望快速采用常见构建方式：选择“平台构建模板”。

平台会根据仓库文件推荐模板，并允许在保存前预览生成结果。模板不会修改代码仓库；项目使用非默认版本、命令或产物路径时，请按实际情况调整参数。

## Next.js 模板为什么构建失败？

Next.js 服务模板要求启用 standalone 输出：

```ts
const nextConfig = {
  output: 'standalone',
}

export default nextConfig
```

平台不会修改仓库配置。缺少该设置时，构建会直接失败。配置文件格式和更多说明见 [Next.js standalone 输出文档](https://nextjs.org/docs/app/api-reference/config/next-config-js/output)。

运行多个副本时，还需要按 [Next.js 自托管指南](https://nextjs.org/docs/app/guides/self-hosting)评估缓存共享、Server Actions 密钥和版本一致性。

## 构建为什么无法下载依赖或基础镜像？

先查看构建日志末尾，再检查构建网络、DNS 和所用镜像源。使用内网 npm、Go Proxy 或镜像站时，需要在平台的构建网络设置中放行对应地址。不要把长期凭据直接写入 Dockerfile 或普通构建参数。

## 构建为什么因资源不足退出？

在部署配置中提高构建任务的 CPU、内存或超时时间，然后重新构建。应用运行资源和构建任务资源是两组独立设置。
