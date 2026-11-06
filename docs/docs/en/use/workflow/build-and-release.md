# Build and Release

## Build from source

A build reads the bound repository, creates an image from the Dockerfile, and pushes it to the registry. Before starting, confirm:

- The Git credential can read the repository and branch.
- The registry credential includes push permission.
- Build arguments do not contain plaintext Secrets; use secret variables instead.

If an explicitly referenced push credential or the build's deployment target no longer exists, is unavailable to the current user, or cannot be queried, the build fails instead of selecting another credential or a default target. Repair the reference before rebuilding.

Read the build log until the task reaches a succeeded, failed, timed-out, or canceled terminal state. Failed, timed-out, and canceled builds show a reason summary under their status in the build list; open **Logs** for the full output. Use unique, traceable image tags for production.

Builds have two time limits. A build that runs longer than its build timeout (from the deployment configuration or `BUILD_JOB_TIMEOUT_SECONDS`) is marked as timed out; the platform deletes its build job and settles the time used. A build still queued after twice its build timeout is also marked as timed out, with a queue-wait reason. Builds cannot be triggered while the project space or deployment configuration is being deleted.

## Release

A Release deploys the selected image to the runtime cluster. After creating one, confirm:

1. The Release reaches its succeeded terminal state.
2. Ready replicas match desired replicas, for example `2/2`.
3. Container logs and health probes are healthy.
4. The Service is reachable inside the cluster.

> “Release succeeded” describes the workflow history; it does not guarantee that the application is still healthy. Current status comes from the cluster. If the cluster is unreachable, Luna DevOps reports it as unavailable instead of showing stale data.

The dashboard, application list, and application details share one runtime status. It only counts deployments that are enabled and have at least one succeeded release, and shows the status that needs the most attention among them. Disabled deployments show **Disabled** and deployments without a succeeded release show **Not deployed**; neither affects the application's overall status. **Current version** is the most recently finished succeeded release on an existing deployment.

When a release fails or is canceled, the **Current version** column shows a reason summary next to its status. Select **View logs** for the full release output.

### Cancel a release

A pending or running release can be canceled from the deployment configuration's action menu with **Cancel release**; this requires release permission. The platform stops the remaining steps and shows the release as canceled.

- Canceling does not roll back resources already applied to the cluster; use **Rollback** to restore the previous version.
- Canceling an already canceled release changes nothing; succeeded or failed releases cannot be canceled.

A release that has not finished within its total time limit (twice the larger of `DEPLOY_ROLLOUT_TIMEOUT_SECONDS` and 30 minutes, 1 hour by default, counted from its start or, while still pending, from its creation) is marked as failed with a time-limit reason. This also ends releases while the runtime cluster stays unreachable.

## Automated delivery

Enable build-to-release automation or repository webhooks only after the first manual flow works end to end.

Repository webhooks trigger builds as the owner of the code repository account and re-check on every push that the owner is still a project-space member allowed to trigger builds, that the project space is not being deleted, and that the balance policy allows new builds. When a check fails, no build is created and the event center records a **Webhook build rejected** event with a reason code. A redelivered event with the same delivery ID from the Git platform creates at most one build.
