# How Platform Features Work Together

Luna DevOps connects source code, images, clusters, and routes through one delivery path:

```text
Project → Application → Deployment target → Build → Release → Route
```

For a first deployment, follow [Deploy a Web Project](/en/start/first-project).

## 1. Projects

A project is the boundary for teams, permissions, and resources. Applications, builds, releases, and routes belong to a project. Projects are commonly divided by product, team, or customer.

Select members from existing platform users and grant the minimum role required for their work.

## 2. Applications

An application is an independently deployable service. One repository may contain separate API, web, and worker applications.

The application topology shows current workloads and dependencies. Projects can also record relationships between applications. A service reference may change the source service's configuration on its next release. Remove references before deleting an application that other services use.

The project application list shows runtime status and ready versus desired replicas separately for each deployment stage, such as development, test, staging, and production. It does not merge replicas from different stages into one total. Applications without a runtime workload are marked as not deployed. This status is observed from the runtime cluster; an unreachable upstream is shown as unavailable instead of reusing stale state.

## 3. Deployment targets

A deployment target defines where an application comes from, how it runs, and which cluster receives it. For the first target, confirm:

- Source repository or existing image.
- Environment stage and runtime cluster.
- Registry and image reference.
- Service ports, CPU, memory, and replicas.
- Environment variables, secrets, files, and volumes.
- Whether a successful build should release automatically.

When the stage is omitted for a new deployment target, the platform uses `dev`. A stage is immutable after creation; `sys-*` stages are reserved for existing platform-managed components.

Consider validating runtime and routing with an existing image before adding a Git provider and automated builds.

## 4. Builds and releases

A build creates an image from source. A release deploys a selected image to the runtime cluster. After a release, check its status, workloads, logs, and health instead of treating task submission as completion.

Runtime status on the deployment page uses `n/m` from one Kubernetes observation for ready and desired replicas. For example, `2/3` means that two of three desired replicas are ready. Release status reports only the historical Release workflow result and does not include current replica counts; a successful release does not by itself mean that a runtime instance is currently ready.

The deployment target list shows runtime specs as `CPU · memory` and uses the **Domain** column for the Kubernetes Service name that is directly reachable inside the same namespace. Choose **View details** from the row's overflow menu to open a right-side panel with the same-namespace Service, cross-namespace FQDN, project addresses, live resource usage, and current release information.

With autoscaling enabled, the runtime cost shown in the form is a baseline estimate based on configured replicas, not a live charge. HPA can change the live desired replica count. An HPA minimum of `0` is supported; the neutral “Scaled to zero · 0/0” state means the workload was observed but currently has no runtime instance, rather than being green and ready. When observation is unavailable, the page omits `0/0` so missing data is not mistaken for a real zero-replica observation.

Configuration or image changes require a new release. Reusing an image tag makes version verification and rollback harder; use traceable, unique tags in production.

When editing a deploy config with live instances, changing replicas or resources, the image, Service/HPA settings, runtime configuration, volumes, advanced Kubernetes settings, or deployment-phase hooks adds an inline redeploy notice and the **Save only** and **Save and redeploy** actions. Settings that do not change the current instances—such as the display name, build parameters, automatic deployment and approval policies, or Web Console access—only need to be saved and do not show the redeploy action. The platform decides whether instances exist from the runtime cluster's live desired replica count; scaled-to-zero and not-yet-deployed configs do not show this notice.

## Advanced runtime settings

Most services can use the defaults. Configure health probes, startup commands, StatefulSets, autoscaling, scheduling, security contexts, sidecars, and advanced storage only when the application requires them.

Ordinary environment variables and ConfigMap references use one `KEY=VALUE` entry per line. The API explicitly classifies runtime environment variables as `public` or `secret`: public values go only to a ConfigMap, while secret values go only to encrypted storage. The platform does not guess sensitivity from a key name or value and does not block any key from being saved as public. A value explicitly saved as `public` is ordinary configuration and can be returned to authorized clients; choose a runtime secret yourself when encryption and non-disclosure are required. A key may temporarily retain both a public value and a secret; the secret always takes precedence in a deployed workload, and the retained public value becomes effective again after the secret is cleared.

Runtime secrets are managed separately from ordinary configuration and secret files. Save the target or runtime configuration set first, then use the key/value rows under **Runtime secrets** to add, replace, generate, or clear a key. Secret buttons perform only the selected secret operation; they do not submit or close the surrounding deployment form. Leaving a replacement blank retains the existing value, while deletion requires the explicit **Clear** action. Each secret change either succeeds completely or rolls back completely; after a successful replacement or clear, the platform removes the old secret when it is no longer referenced. Values remain masked and are never returned as plaintext by the API, personal access tokens, or the AI assistant.

Create or reference persistent data in the project [volume center](./project-volumes.md), then select its stable project volume from a deployment target. Fill each mount by identifier, source, path, and source details; these fields stack automatically on narrow screens and in narrow configuration dialogs. PVC storage properties are generally chosen at creation time; `emptyDir` data disappears with its Pod and is not included in exports.

Deleting an application or deployment target only unbinds its project volumes. It does not change project ownership or automatically delete a PVC. A managed PVC is deleted only from the volume details after impact review and explicit permanent-deletion confirmation; [export an archive](./volume-transfer.md) first when the data may still be needed.

Shared configuration can follow the latest value or use a snapshot. Deployment hooks suit one-time tasks such as database migrations. A failed hook can stop a release, so scripts should be repeatable and have a rollback plan.

## Web Console and data export

Web Console access depends on project settings, the current sign-in session, and account permission. These conditions are rechecked while connected. Terminal use is audited; avoid printing or pasting unnecessary secrets.

A data export is an asynchronous transfer created from the volume details. It requires project Owner or Admin permission. The browser exchanges authorization for a short-lived, single-use download ticket bound to the current user, sign-in session, and transfer, and supports resuming. `emptyDir` is not included, and the transfer must reach Succeeded before the backup is complete. See [Import and Export Volumes](./volume-transfer.md).

## Routes

A route connects a domain, path, and TLS mode to an application service. After creation, check its status, DNS, Service port, and application health.

### Choose a TLS mode

| Scenario | Recommended mode |
| --- | --- |
| A CDN or upstream proxy already terminates HTTPS | TLS terminated by upstream proxy |
| The cluster Gateway uses an existing certificate | TLS terminated by Gateway |
| cert-manager uses HTTP-01 | HTTP Challenge certificate |
| cert-manager uses a DNS-01 wildcard certificate | Configure the wildcard certificate on the cluster |

The platform uses Gateway, Issuer, and certificate resources already configured in the runtime cluster. It does not create ACME accounts or DNS provider credentials.

If an OIDC application generates an incorrect `http` callback, check whether the CDN, reverse proxy, and Gateway preserve the external protocol. Forwarded-header or TLS changes can affect every route and should be evaluated by an administrator.
