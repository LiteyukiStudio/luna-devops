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

The project application list shows ready replicas versus desired replicas. Applications without a runtime workload are marked as not deployed. This status is observed from the runtime cluster; an unreachable upstream is shown as unavailable instead of reusing stale state.

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

Runtime and release status on the deployment page use `n/m` for ready and desired replicas. For example, `2/3` means that two of three desired replicas are ready.

With autoscaling enabled, the runtime cost shown in the form is a baseline estimate based on configured replicas, not a live charge. HPA can change the live desired replica count. An HPA minimum of `0` is supported; `0/0` means the workload is observed and currently scaled to zero.

Configuration or image changes require a new release. Reusing an image tag makes version verification and rollback harder; use traceable, unique tags in production.

## Advanced runtime settings

Most services can use the defaults. Configure health probes, startup commands, StatefulSets, autoscaling, scheduling, security contexts, sidecars, and advanced storage only when the application requires them.

Ordinary environment variables and ConfigMap references use one `KEY=VALUE` entry per line in the form and are returned by the API as JSON objects. Runtime secrets are managed separately from ordinary configuration and secret files. Save the target first, then use **Runtime secrets** to add, replace, generate, or clear a key. Values remain masked and are never returned as plaintext by the API, personal access tokens, or the AI assistant.

Create or reference persistent data in the project [volume center](./project-volumes.md), then select its stable project volume from a deployment target. PVC storage properties are generally chosen at creation time; `emptyDir` data disappears with its Pod and is not included in exports.

Deleting an application or deployment target only unbinds its project volumes. It does not change project ownership or automatically delete a PVC. A managed PVC is deleted only from the volume details after impact review, explicit permanent deletion, and step-up verification; [export an archive](./volume-transfer.md) first when the data may still be needed.

Shared configuration can follow the latest value or use a snapshot. Deployment hooks suit one-time tasks such as database migrations. A failed hook can stop a release, so scripts should be repeatable and have a rollback plan.

## Web Console and data export

Web Console access depends on project settings, account permission, and MFA policy. Terminal use is audited; avoid printing or pasting unnecessary secrets.

A data export is an asynchronous transfer created from the volume details. It requires project Owner or Admin permission and any required step-up verification. The browser exchanges authorization for a short-lived download session and supports resuming. `emptyDir` is not included, and the transfer must reach Succeeded before the backup is complete. See [Import and Export Volumes](./volume-transfer.md).

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
