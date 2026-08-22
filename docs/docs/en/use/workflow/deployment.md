# Create a Deployment

A deployment defines **where the application comes from, where it runs, and how it runs**. An application can have only one deployment for each stage.

## Required settings

- Stage: `dev`, `test`, `staging`, or `prod`. It cannot be changed after creation.
- Target runtime cluster.
- Source: existing image or source repository.
- Container port, per-replica CPU quota, per-replica memory quota, and replicas.
- Image pull policy and required health checks.

## Choose a source

**Existing image** is the fastest route. Enter the full image reference, confirm that the target cluster can pull it, save, and create a Release.

**Source repository** requires a repository binding, branch, and Dockerfile, plus a registry with push credentials. Build first, then release the generated image.

Open advanced settings only when the application needs custom commands, autoscaling, StatefulSet, scheduling, security contexts, sidecars, or deployment hooks.

Changes to the image, resources, Service, volumes, or advanced runtime settings require a new Release to affect running instances. When the page offers **Save and redeploy**, review the impact before using it.

CPU and memory inputs are quotas, not Kubernetes requests or limits. Luna DevOps derives the actual fields from the selected runtime cluster policy; a `0%` policy value omits the corresponding field. A deployment cannot override its CPU or memory limit separately.
