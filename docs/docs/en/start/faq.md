# FAQ

## Should I use a repository Dockerfile or a platform template?

- If the project already maintains a Dockerfile, choose **Repository Dockerfile**.
- If it has no Dockerfile or you want a common build setup quickly, choose **Platform build template**.

The platform recommends templates from repository files and lets you preview the result before saving. A template does not modify the repository. Adjust versions, commands, and artifact paths when the project differs from the defaults.

## Why does the Next.js template fail to build?

The Next.js service template requires standalone output:

```ts
const nextConfig = {
  output: 'standalone',
}

export default nextConfig
```

The platform does not change repository configuration, so the build fails when this setting is missing. See the [Next.js standalone output documentation](https://nextjs.org/docs/app/api-reference/config/next-config-js/output) for supported configuration formats.

For multiple replicas, also review cache sharing, Server Actions keys, and version consistency in the [Next.js self-hosting guide](https://nextjs.org/docs/app/guides/self-hosting).

## Why can the build not download dependencies or base images?

Check the end of the build log, then verify build networking, DNS, and registry configuration. Internal npm registries, Go proxies, or image registries must be allowed by the platform build network settings. Do not put long-lived credentials in a Dockerfile or ordinary build arguments.

## Why does the build exit because of insufficient resources?

Increase the build CPU, memory, or timeout in the deployment target, then build again. Runtime resources and build-job resources are configured separately.
