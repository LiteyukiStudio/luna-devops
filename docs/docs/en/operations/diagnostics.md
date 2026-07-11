# Status and Troubleshooting

Do not start by retrying everything. First decide whether the failure is in build, release, application runtime, or the access route, then inspect the matching record. Liteyuki DevOps keeps those stages close together so you can follow the same delivery context.

## Start with the event center

When the first question is "what just happened?", open **Events** from the sidebar. It presents build, release, deployment hook, access-route, and certificate state changes in time order.

Select multiple project spaces, applications, deployment targets, categories, severities, and results, then combine them with a time range. Application and deployment-target options follow the selected parent resources, and stale child filters are removed when their parent is deselected. Event details show the failure summary, related resources, actor, and direct links to the relevant build, release, or access page. Regular users only see events from project spaces they can access. Platform administrators can switch between **Related to me** and **All events**.

## Build did not succeed

Open the failed build record and check:

- Dockerfile path.
- Build context.
- Dependency download failures.
- Registry push credentials.

When a build record shows `kubernetes build job failed`, the platform enriches the message with the build Pod status and recent Kubernetes Events. Common fields include `executor terminated`, `exitCode=137`, `OOMKilled`, `Evicted`, and `BackOff`. `exitCode=137` / `OOMKilled` usually means the build environment ran out of memory; increase the build environment size in the deployment target and retry.

If Git and registry connections are not ready yet, deploy an existing image first to verify the second half of the delivery path.

## Release did not succeed

Open the Release status and deployment logs, then check:

- Image exists.
- Runtime cluster is reachable.
- Image pull credential is correct.
- Service port matches the real application port.
- Environment variables, secrets, and config files match application expectations.

## Route is not reachable

Check in this order:

- Domain resolves to the right entrypoint.
- Gateway API CRDs are installed, and GatewayClass/Gateway exist.
- HTTPRoute is Accepted, ResolvedRefs, and Programmed.
- Service points to the correct port.
- TLS settings match the gateway.
- Service endpoints have ready Pods.

For local test domains, start with hosts or `curl --resolve` before changing public DNS.

## Recovery suggestions

- Wrong config: edit the deployment target and release again.
- Wrong image: select the right image and create a new Release.
- App issue: inspect runtime logs before restart or rollback.
- Route issue: check route status first, then the cluster gateway.
