# Next Steps

After the platform is running, you do not need every external integration immediately. Follow this order to avoid loops.

## 1. Configure a runtime cluster

The runtime cluster is where applications are deployed. It can be Kubernetes or a lightweight K3s cluster.

For the first integration, prepare a test cluster and make sure its kubeconfig can be reached from the API and worker containers.

Before deleting a runtime cluster, migrate or delete deploy configs that reference it. The platform does not automatically delete deploy configs when a cluster is deleted.

## 2. Configure a registry

The registry stores build outputs and can also provide existing images for deployment.

If you only want to explore deployment, start with an existing image. Add push credentials when you are ready for the full build path.

## 3. Create a deployment target

A deployment target answers how this application should ship:

- Existing image or repository build.
- Runtime cluster.
- Stage for this deployment target, such as development, test, staging, or production.
- Runtime replicas, CPU, and memory.
- Build Job CPU and memory.
- Service port.
- Environment variables, secrets, and config files.

## 5. Create a route

A route connects domain, path, TLS, and backend service. After creating one, check its status in the console, then verify it with a browser or `curl`.
