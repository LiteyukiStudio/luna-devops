# Daily Delivery

Luna DevOps uses one clear path to deliver an application to a cluster:

```text
Project space → Application → Deployment → Build or select image → Release → Route
```

> **A project space is a logical namespace.** It isolates members, applications, and resources for a product or team, but it is not the same as a Kubernetes Namespace.

## Shortest path

1. [Create a project space and application](./project-space-and-apps).
2. [Create a deployment](./deployment) and select the cluster and runtime resources.
3. Choose the delivery source:
   - Existing image: enter the image and release it directly.
   - Source only: connect the repository, [build an image, and release it](./build-and-release).
4. Confirm that the Release and workload become ready.
5. [Create a route](./routes) when the application needs a public or custom domain.

## Configure when needed

- Environment variables, files, and Secrets: [Configuration and Secrets](./configuration-and-secrets)
- Move deployment settings: [Import and Export Deployment Config](./deployment-config-transfer)
- Preserve databases and uploaded files: [Runtime Data Persistence](./runtime-data-persistence)

For a first deployment, validate cluster, release, and route with an existing image before adding source builds and automation. This makes failures easier to locate.
