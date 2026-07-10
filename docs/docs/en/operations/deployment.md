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

Advanced Kubernetes config is collapsed by default and should be used only when an image needs specific runtime conditions. Current support includes:

- Container startup: override `command` / `args`, set `imagePullPolicy`, and configure readiness, liveness, and startup probes.
- Workload: Deployment is the default. StatefulSet can be selected in advanced settings when the app needs stable Pod names, ordered rollout, or stateful workload semantics. The platform handles rendering, HPA target refs, runtime checks, restart, and cleanup according to the selected workload.
- Lifecycle: configure `postStart` and `preStop` through Kubernetes Lifecycle JSON.
- Init and helper containers: configure initContainers and sidecars through a constrained Container JSON array. The platform trims risky fields such as `hostPort`, external `envFrom`, external Secret references, and privilege escalation, then injects the current deploy config's ConfigMap/Secret envFrom.
- Resource limits: set CPU / memory limits separately from requests. Empty values do not set limits.
- Auto scaling: when HPA is enabled, the platform creates an `autoscaling/v2 HorizontalPodAutoscaler` and scales replicas by CPU or memory average utilization targets. Optional HPA behavior JSON can tune scaling speed and stabilization windows. The runtime cluster needs metrics-server or an equivalent metrics API.
- Security context: set `runAsUser`, `runAsGroup`, `fsGroup`, `fsGroupChangePolicy`, read-only root filesystem, `allowPrivilegeEscalation`, and capabilities. Images such as OpenList that require a fixed UID/GID for a writable data directory can be handled here.
- Scheduling: set `nodeSelector`, `tolerations`, basic `affinity`, `topologySpreadConstraints`, and `priorityClassName`.
- Service and storage: Service remains ClusterIP by default. The advanced section can set Service type, annotations, `appProtocol`, session affinity, and external traffic policy. When runtime data is enabled, it can also set PVC `storageClassName`, `accessMode`, and `volumeMode`. Data volume sources support platform-managed PVCs, existing PVCs, and temporary `emptyDir`.

Complex structured fields use native Kubernetes JSON, such as Probe, Toleration, Affinity, and TopologySpreadConstraint. Simple key-value fields accept either a JSON object or `KEY=VALUE` lines.

PVC `storageClassName` and `accessMode` are written only when the data volume is first created. Existing PVCs are only resized; the platform does not migrate or recreate storage automatically.
`emptyDir` data is destroyed with the Pod and is not exported by the platform. Existing PVCs can be exported, but the platform does not manage their capacity or lifecycle.
The current StatefulSet path reuses platform-managed PVCs, existing PVCs, and emptyDir. It does not generate `volumeClaimTemplates`; per-replica persistent volumes will be designed in a later advanced orchestration phase.

Repository webhooks belong to application repository bindings. When the Git platform sends a push/tag event, the platform finds enabled, active deployment targets under the same application that use that repository binding, then creates build runs according to their branch and tag patterns. Deployment targets do not create separate external webhooks, so each repository event enters the platform once.

When selecting a repository Dockerfile, the platform tries to read its `EXPOSE` instructions and fills the service port list automatically. If the service has multiple HTTP ports, add them on the deployment target. Gateway routes must choose one of the exposed ports as their target port.

When build variables or runtime config sets are deleted, the platform removes those references from deployment targets that still point to them, so deployment targets do not keep stale configuration IDs.

When a deployment target attaches a project-space runtime config set, it can use two modes. A live reference reads the latest shared config on the next release and prompts redeploys after the shared config changes. A snapshot freezes the shared config when the deployment target is saved, so later shared config updates do not affect that deployment target. Secrets remain in the platform secret store in both modes; deployment targets only store secret references or frozen secret references.

Project-space hooks are reusable script definitions. They only run after a deployment target binds them in its “Deployment hooks” section. A deployment target can bind the same project hook to build, image push, pre-deployment, or post-deployment phases and control the execution order locally. Pre-deployment hooks run after runtime ConfigMap/Secret resources are written and before the application Deployment rolls out, which fits database migrations, seed commands, or one-shot repair commands that must complete before the app container starts.

Deployment-phase hooks run as Kubernetes Jobs. The platform stores hook run records and logs; the in-cluster Hook Job/Pod is kept only briefly for troubleshooting. Successful hooks are cleaned up after 5 minutes by default, and failed hooks after 24 hours.

When deleting a deployment target, the platform first deletes routes bound to that target, then cleans up the Kubernetes workload, Service, and optional data volumes. This prevents routes from pointing at a service that no longer exists.

Gateway routes are enabled by default when created. To temporarily stop public access without losing the domain config, disable the route; the platform keeps the config and removes the runtime HTTPRoute, then reapplies it when enabled again.

## Builds and releases

Builds create images. Releases deploy images to runtime clusters.

Every new Release updates the Kubernetes Pod Template release fingerprint, so a rollout is triggered even when the target image tag stays the same. By default, config-only changes restart Pods without forcing an image pull. When a Release comes from a new build artifact and the image tag stays the same, the platform temporarily uses `imagePullPolicy: Always` to avoid stale node cache issues with fixed tags.

If the remote image content changed but the tag did not, choose “Pull latest image and deploy” from the deploy config actions menu. This creates a new Release and forces an image pull for that rollout.

When the platform creates a Kubernetes Deployment, the selector only identifies the workload for the current deploy config and remains stable across later releases. Project, application, environment, and Release ownership metadata is written to resource labels or Pod Template annotations instead of changing the selector. This avoids update failures caused by Kubernetes `spec.selector` immutability.

For a first run, deploy an existing image before wiring Git providers and automated builds.

The application deployment list refreshes runtime metrics every second through SSE. Metrics come from the Kubernetes standard `metrics.k8s.io` Pod Metrics API, so the runtime cluster needs metrics-server installed. CPU percentage and memory usage are calculated from current usage divided by “environment size × replicas”. If the cluster does not expose metrics, the page shows metrics unavailable.

## Routes

Routes connect domain, path, TLS, and backend service. After creating one, the platform shows apply status and checks so you can verify the service is reachable.

Domain suffixes come from the deployment target's runtime cluster. Administrators can maintain multiple suffixes on a cluster, and each access route selects exactly one. Short host prefixes and generated default domains use the selected suffix, while full custom domains can still be entered directly.

The runtime cluster's external access scheme and external access port only control the URLs the console displays and opens. If an outer CDN or reverse proxy already terminates HTTPS, set the external access scheme to `https` and choose upstream TLS termination; the platform will bind HTTPRoutes to the internal HTTP listener and will not request an in-cluster certificate just because the displayed URL is HTTPS.

If HTTPS terminates on the cluster Gateway itself, set the runtime cluster's external TLS mode to Gateway termination and configure an existing Kubernetes TLS Secret. When access routes are applied, the platform ensures the shared Gateway HTTPS listener references that Secret, and HTTPRoutes bind to the HTTPS listener by default.

The "HTTP Challenge certificate" route mode depends on the runtime cluster's cert-manager settings. The platform creates a Certificate and references the Secret produced by that Certificate from the shared Gateway HTTPS listener. The Worker periodically synchronizes the cert-manager Ready condition, failure message, and `notAfter`. The application Routes list shows none, applying, enabled, failed, or expired beside the TLS mode; hover the status to inspect the failure reason, expiry, and referenced Issuer.

The platform only references the runtime cluster's `Issuer` or `ClusterIssuer`; it does not create an ACME account. The default name `letsencrypt-http01` is only an Issuer resource name. The actual CA, ACME email, account Secret, and HTTP-01 solver are defined by that Issuer's `spec.acme` configuration.

If DNS-01 wildcard certificates are enabled on the runtime cluster, the platform also attaches the wildcard certificate Secret to the HTTPS listener. This fits clusters where an outer gateway forwards traffic to internal ports, public HTTP-01 ingress is unavailable, or one certificate should cover a whole domain suffix.

Access routes are backed by Kubernetes Gateway API. A runtime cluster owns one platform-managed `Gateway`, and each access route creates an `HTTPRoute` in the project namespace that forwards to the deployment target `Service`. Install the Gateway API CRDs before enabling routes. Traefik clusters also need `--providers.kubernetesGateway`.

When an access route is created or enabled, the platform checks that the deployment target's Kubernetes `Service` and selected port already exist. If an administrator manually deleted the Service, the platform asks the user to redeploy the deployment target to restore runtime resources instead of creating the Service from the access-route flow. This keeps routing separate from workload reconciliation and avoids silently repairing drift from stale deploy config.

Runtime clusters can define default Gateway settings, including controller type, GatewayClass, Gateway name/namespace, external TLS mode, forwarded header policy, trusted proxy CIDRs, and default request/response headers. The route form shows only the common basics by default: deploy config, domain, path, service port, and TLS. Expand advanced settings only when you need to override a route's Parent Gateway, path match type, request/response headers, URL rewrite, redirect, or backend weight.

For a chain such as `CDN HTTPS -> Nginx HTTP -> Traefik HTTP -> Pod`, prefer configuring Traefik entryPoint `forwardedHeaders.trustedIPs` to trust the upstream proxy and forward `X-Forwarded-Proto=https`. Apps such as Logto/OIDC providers may generate the wrong issuer or redirect URL if the backend sees `http`; as a fallback, choose upstream TLS + overwrite on the runtime cluster so the platform injects `X-Forwarded-Proto=https` and `X-Forwarded-Port=443` through HTTPRoute RequestHeaderModifier.
