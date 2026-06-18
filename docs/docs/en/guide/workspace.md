# Next Steps

After the platform is running, you do not need every external integration immediately. Follow this order to avoid loops.

## 1. Configure a runtime cluster

The runtime cluster is where applications are deployed. It can be Kubernetes or a lightweight K3s cluster.

For the first integration, prepare a test cluster and make sure its kubeconfig can be reached from the API and worker containers.

Before deleting a runtime cluster, migrate or delete environments that reference it. The platform does not automatically delete environments when a cluster is deleted.

## 2. Configure a registry

The registry stores build outputs and can also provide existing images for deployment.

If you only want to explore deployment, start with an existing image. Add push credentials when you are ready for the full build path.

## 3. Create an environment

Environments usually map to `dev`, `staging`, and `prod`. When a project space is created, the platform creates a `prod` environment by default with 1 replica, 0.5 CPU core, and 0.5 GiB memory; add `dev` or `staging` later when you need a multi-stage delivery flow.

After an environment is bound to a runtime cluster, deployment targets know where to publish.

Before deleting an environment, clean up deployment targets, routes, and releases that reference it, so the delivery path does not keep stale references.

## 4. Create a deployment target

A deployment target answers how this application should ship:

- Existing image or repository build.
- Target environment.
- Service port.
- Environment variables, secrets, and config files.

## 5. Create a route

A route connects domain, path, TLS, and backend service. After creating one, check its status in the console, then verify it with a browser or `curl`.
