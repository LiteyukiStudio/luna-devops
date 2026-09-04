# Build and Release

## Build from source

A build reads the bound repository, creates an image from the Dockerfile, and pushes it to the registry. Before starting, confirm:

- The Git credential can read the repository and branch.
- The registry credential includes push permission.
- Build arguments do not contain plaintext Secrets; use secret variables instead.

If an explicitly referenced push credential or the build's deployment target no longer exists, is unavailable to the current user, or cannot be queried, the build fails instead of selecting another credential or a default target. Repair the reference before rebuilding.

Read the build log until the task reaches a succeeded or failed terminal state. Use unique, traceable image tags for production.

## Release

A Release deploys the selected image to the runtime cluster. After creating one, confirm:

1. The Release reaches its succeeded terminal state.
2. Ready replicas match desired replicas, for example `2/2`.
3. Container logs and health probes are healthy.
4. The Service is reachable inside the cluster.

> “Release succeeded” describes the workflow history; it does not guarantee that the application is still healthy. Current status comes from the cluster. If the cluster is unreachable, Luna DevOps reports it as unavailable instead of showing stale data.

Enable build-to-release automation or repository webhooks only after the first manual flow works end to end.
