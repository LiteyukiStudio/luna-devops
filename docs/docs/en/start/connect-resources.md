# Connect Cluster and Registry

Once the console opens, resist the urge to connect everything at once. Prepare the following pieces in order so that a failure is easy to locate.

```text
Runtime cluster -> Registry -> Deployment target -> Route
```

## 1. Configure a runtime cluster

A runtime cluster is where applications actually run. It can be Kubernetes or the lighter K3s distribution.

For the first integration, prepare a test cluster and make sure its kubeconfig can be reached from the API and worker containers.

Before deleting a runtime cluster, migrate or delete deployment targets that reference it. The platform does not automatically delete deployment targets when a cluster is deleted.

## 2. Configure a registry

The registry stores build outputs and can also provide existing images for deployment.

If you only want to explore deployment, start with an existing image. Add push credentials when you are ready for the full build path.

## 3. Create a deployment target

A deployment target describes how this application is built and run:

- Existing image or repository build.
- Runtime cluster.
- Stage, such as development, test, staging, or production.
- Runtime replicas, CPU, and memory.
- Build Job CPU and memory.
- Service port.
- Environment variables, secrets, config files, and data volumes.

If you only want to verify the platform first, use an existing image. After the cluster and route work, connect Git providers and automated builds.

## 4. Create a route

A route points a domain, path, and TLS policy at an already deployed service. Check its status first, then send a real request with a browser or `curl`.

## 5. Enable automation later

After the first manual deployment works, enable automation one step at a time:

- Git providers and Git account authorization.
- Repository binding and webhooks.
- Automatic builds.
- Auto deploy after successful builds.
- Custom domains and HTTPS.
