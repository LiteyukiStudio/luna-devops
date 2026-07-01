# Feature Map

Liteyuki DevOps connects code, images, clusters, and routes into one delivery path. You do not need to understand every underlying component before using it.

For a click-through walkthrough, see [Deploy a Web Project](/en/operations/deploy-web-project). It uses `snowykami/neo-blog` as the example and covers project space, applications, deployment targets, builds, releases, and the public route.

## Project spaces

A project space is the boundary for teams and resources. Members, applications, deploy configs, build records, releases, and routes all belong to a project space.

When adding project space members, Owner/Admin users search platform users by name or email, select one or more suggestions, and then add them. Free-form email text is not submitted as a member by itself.

Common patterns:

- One product per project space.
- One small team per project space.
- One customer or demo environment per project space.

## Applications

An application is a deployable service. One repository can map to multiple applications, such as API, Web, and Worker in a monorepo.

The application stores basic service information. Build source, image, environment variables, and release policy live in deployment targets.

The application overview summarizes runtime specs by deployment target, including replicas, CPU, memory, and enabled data volume capacity, so teams can quickly inspect application resource usage.

## Deployment targets

A deployment target answers how an application should ship:

- Build from a repository or use an existing image.
- Publish to which environment.
- Stage for this deployment target, such as development, test, staging, or production.
- Use which registry.
- Listen on which service ports. A deployment target can expose multiple service ports; the first port is the default, for example `8080` for HTTP traffic and `9001` for Prometheus metrics.
- Build spec and timeout. The default build timeout is 30 minutes, and it can be adjusted on the deployment target or temporarily overridden for a manual build.
- Auto release after a successful build or not.

Repository webhooks belong to application repository bindings. When the Git platform sends a push/tag event, the platform finds enabled, active deployment targets under the same application that use that repository binding, then creates build runs according to their branch and tag patterns. Deployment targets do not create separate external webhooks, so each repository event enters the platform once.

When selecting a repository Dockerfile, the platform tries to read its `EXPOSE` instructions and fills the service port list automatically. If the service has multiple HTTP ports, add them on the deployment target. Gateway routes must choose one of the exposed ports as their target port.

When build variables or runtime config sets are deleted, the platform removes those references from deployment targets that still point to them, so deployment targets do not keep stale configuration IDs.

When a deployment target attaches a project-space runtime config set, it can use two modes. A live reference reads the latest shared config on the next release and prompts redeploys after the shared config changes. A snapshot freezes the shared config when the deployment target is saved, so later shared config updates do not affect that deployment target. Secrets remain in the platform secret store in both modes; deployment targets only store secret references or frozen secret references.

When deleting a deployment target, the platform first deletes routes bound to that target, then cleans up the Kubernetes workload, Service, and optional data volumes. This prevents routes from pointing at a service that no longer exists.

Gateway routes are enabled by default when created. To temporarily stop public access without losing the domain config, disable the route; the platform keeps the config and removes the runtime Ingress, then reapplies it when enabled again.

## Builds and releases

Builds create images. Releases deploy images to runtime clusters.

Every new Release updates the Kubernetes Pod Template release fingerprint, so a rollout is triggered even when the target image tag stays the same. By default, config-only changes restart Pods without forcing an image pull. When a Release comes from a new build artifact and the image tag stays the same, the platform temporarily uses `imagePullPolicy: Always` to avoid stale node cache issues with fixed tags.

If the remote image content changed but the tag did not, choose “Pull latest image and deploy” from the deploy config actions menu. This creates a new Release and forces an image pull for that rollout.

When the platform creates a Kubernetes Deployment, the selector only identifies the workload for the current deploy config and remains stable across later releases. Project, application, environment, and Release ownership metadata is written to resource labels or Pod Template annotations instead of changing the selector. This avoids update failures caused by Kubernetes `spec.selector` immutability.

For a first run, deploy an existing image before wiring Git providers and automated builds.

The application deployment list refreshes runtime metrics every second through SSE. Metrics come from the Kubernetes standard `metrics.k8s.io` Pod Metrics API, so the runtime cluster needs metrics-server installed. CPU percentage and memory usage are calculated from current usage divided by “environment size × replicas”. If the cluster does not expose metrics, the page shows metrics unavailable.

## Routes

Routes connect domain, path, TLS, and backend service. After creating one, the platform shows apply status and checks so you can verify the service is reachable.

The site-level “public route link scheme” only controls whether the console displays and opens route links with `http` or `https`. If an outer CDN or reverse proxy already terminates HTTPS, set it to `https` while keeping the route TLS mode as HTTP-only, so the platform does not request an in-cluster certificate.
