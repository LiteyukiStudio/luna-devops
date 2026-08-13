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

## 3. Deployment targets

A deployment target defines where an application comes from, how it runs, and which cluster receives it. For the first target, confirm:

- Source repository or existing image.
- Environment stage and runtime cluster.
- Registry and image reference.
- Service ports, CPU, memory, and replicas.
- Environment variables, secrets, files, and volumes.
- Whether a successful build should release automatically.

Consider validating runtime and routing with an existing image before adding a Git provider and automated builds.

## 4. Builds and releases

A build creates an image from source. A release deploys a selected image to the runtime cluster. After a release, check its status, workloads, logs, and health instead of treating task submission as completion.

Configuration or image changes require a new release. Reusing an image tag makes version verification and rollback harder; use traceable, unique tags in production.

## Advanced runtime settings

Most services can use the defaults. Configure health probes, startup commands, StatefulSets, autoscaling, scheduling, security contexts, sidecars, and advanced storage only when the application requires them.

PVC storage properties are generally chosen at creation time; do not assume automatic migration. `emptyDir` data disappears with its Pod and is not included in data exports. Confirm backup and recovery before changing or retaining storage.

Deleting an application retains its managed PVCs by default and registers them as data volumes that can be reclaimed. To restore the data, select the retained volume in a new deployment target on the same runtime cluster. The platform deletes a PVC only after you explicitly choose permanent deletion and confirm the operation; export a backup first when the data may still be needed.

Shared configuration can follow the latest value or use a snapshot. Deployment hooks suit one-time tasks such as database migrations. A failed hook can stop a release, so scripts should be repeatable and have a rollback plan.

## Web Console and data export

Web Console access depends on project settings, account permission, and MFA policy. Terminal use is audited; avoid printing or pasting unnecessary secrets.

Data export requires project Owner or Admin permission and any required step-up verification. It includes managed or attached PVCs, not `emptyDir`. Before export or cleanup, consider file size, sensitive content, and destination storage.

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
