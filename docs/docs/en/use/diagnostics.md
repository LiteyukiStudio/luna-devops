# Status and Troubleshooting

First determine whether a problem is in the build, release, application runtime, or gateway. Avoid repeated retries until the likely cause is known.

## Start with events

Open **Events**. The list defaults to **Related to me**, containing events performed by the current account and events from its project spaces. Platform administrators switch to **All** only for explicit platform-wide troubleshooting. Then narrow the time range, project, application, deployment target, and severity; these filters apply within the current visibility. Event details include a failure summary, related resources, and links to relevant pages.

## Build failures

Open the build record and check:

- Dockerfile path and build context.
- Dependency download availability.
- Registry push credentials.
- Build CPU and memory.

`exitCode=137` or `OOMKilled` usually indicates insufficient memory. Increase build resources before retrying. If Git and a registry are not configured yet, deploy an existing image to validate the release and runtime path first.

If starting or retrying a build returns `build.registry_push_credential_required`, no new BuildRun was created. Add or bind a credential for that build's target registry that is visible to the current user or project and has **push** or **push-pull** usage, then retry. Changing the branch, Dockerfile, or image name does not resolve a missing credential.

## Release failures

Open the release and deployment logs. Confirm that the image exists, the cluster is reachable, pull credentials are valid, and ports, environment variables, secrets, and configuration files match the application.

For an unready application, inspect workload events and container logs to distinguish image pulls, startup failures, health checks, and resource exhaustion.

When a deployment has several replicas, runtime logs and the Web Console connect to one replica at a time. The default **Automatic** option prefers a ready replica of the current release. To inspect a specific replica, such as one that is restarting or not ready, choose it in the **Replica** list. The list shows each replica's readiness, restart count, and whether it belongs to this release or another one. If the selected replica is recreated or scaled away, the page asks you to choose again instead of switching replicas silently. In the CLI, add `pod=<pod name>` to `luna deployment exec` or `luna release exec` to choose a replica.

## An unreachable route

Check DNS, route status, the Service port, available Pods, and TLS configuration in that order. Administrators should also inspect Gateway, HTTPRoute, and certificate status.

For a local test domain, use a hosts entry or `curl --resolve` before changing public DNS.

## Recovery

- Incorrect configuration: update the deployment target and create a release.
- Incorrect image: select the correct image and release it.
- Application failure: inspect logs before restarting or rolling back.
- Route failure: inspect the route before the cluster Gateway.

Rollbacks, restarts, and releases can affect live traffic. Verify the target version, impact, and recovery path before changing production.
