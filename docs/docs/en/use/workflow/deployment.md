# Create a Deployment

A deployment defines **where the application comes from, where it runs, and how it runs**. An application can have only one deployment for each stage.

Select **New deploy config** on the application's **Deployments** tab, or choose **Edit** from a deployment's action menu, to open the dedicated editor page. The page is split into Basics, Source and build, Runtime, Release triggers, Hooks, and Security and scheduling. The section navigation (on the left, or at the top on narrow screens) summarizes each section. If any field fails validation when you save, the navigation marks the affected sections and the bottom action bar can jump to the first error.

## Required settings

- Stage: `dev`, `test`, `staging`, or `prod`. It cannot be changed after creation.
- Target runtime cluster.
- Source: existing image or source repository.
- Container port, per-replica CPU quota, per-replica memory quota, and replicas.
- Image pull policy and required health checks.

The **Idle / Light / Moderate / Heavy / Full** badge beside each runtime cluster is a current pressure overview refreshed about every 10 seconds. Regular users see only the level, not exact cluster capacity or other workload usage. **Unavailable** means Luna DevOps could not complete the observation. The level helps choose a cluster but does not guarantee that Kubernetes can schedule a particular Pod.

## Choose a source

**Existing image** is the fastest route. Enter the full image reference, confirm that the target cluster can pull it, save, and create a Release.

**Source repository** requires a repository binding, branch, and Dockerfile, plus a registry with push credentials. Build first, then release the generated image.

Rarely used options live under each section's advanced settings, which expand automatically and show a count when values are set: startup commands, pull policy, lifecycle, and probes are under **Runtime → Container startup and health checks**; StatefulSet is under **Runtime → Storage**; scheduling rules, security contexts, and auxiliary containers are under **Security and scheduling**; deployment hooks are under **Hooks**. JSON fields are validated before saving.

When autoscaling is enabled, the replica input is replaced by minimum replicas, maximum replicas, and CPU or memory target utilization. Set at least one target utilization.

Set replicas to `0` to retain the deployment configuration and its Kubernetes resources while stopping all Pods. Set replicas back to a positive integer to resume the workload. Switching between zero and a positive replica count applies directly to the current workload without creating a new Release. When autoscaling is enabled, the minimum replica count must be at least `1` because the current CPU and memory resource metrics cannot wake a workload from zero replicas.

Changes to the image, resource quotas, Service, volumes, or advanced runtime settings require a new Release to affect running instances. When the page offers **Save and redeploy**, review the impact before using it.

CPU and memory inputs are quotas, not Kubernetes requests or limits. Luna DevOps derives the actual fields from the selected runtime cluster policy; a `0%` policy value omits the corresponding field. A deployment cannot override its CPU or memory limit separately.
