# Create a Deployment

A deployment defines **where the application comes from, where it runs, and how it runs**. An application can have only one deployment for each stage.

Select **New deploy config** on the application's **Deployments** tab, or choose **Edit** from a deployment's action menu, to open the dedicated editor page. The page is split into Basics and source, Runtime, Release triggers, and Advanced (Kubernetes). The section navigation (on the left, or at the top on narrow screens) summarizes each section. If any field fails validation when you save, the navigation marks the affected sections and the bottom action bar can jump to the first error.

## Required settings

- Stage: `dev`, `test`, `staging`, or `prod`. It cannot be changed after creation.
- Target runtime cluster.
- Source: existing image or source repository.
- Container port, per-replica CPU quota, per-replica memory quota, and replicas.
- Image pull policy and required health checks.

The **Idle / Light / Moderate / Heavy / Full** badge beside each runtime cluster is a current pressure overview refreshed about every 10 seconds. Regular users see only the level, not exact cluster capacity or other workload usage. **Unavailable** means Luna DevOps could not complete the observation. The level helps choose a cluster but does not guarantee that Kubernetes can schedule a particular Pod.

When the runtime cluster is left at the **Platform default cluster** or **current default cluster** option, the deployment runs on the global runtime cluster marked as default. If no cluster is marked as default, the earliest added global runtime cluster is used. Project-space and personal runtime clusters are never used implicitly; select them explicitly in the deployment. The cluster shown on the Deployments tab is the cluster Luna DevOps actually deploys to, observes, and meters. When no runtime cluster is available, the deployment shows **No cluster**.

Turning off **Enable this deploy config** removes the deployment from the application's overall runtime status, but it does not stop Pods that are already running; the Deployments tab still shows their real replica counts. To stop the workload, set replicas to `0`.

## Choose a source

**Existing image** is the fastest route. Enter the full image reference, confirm that the target cluster can pull it, save, and create a Release.

**Code repository** is chosen in **Basics and source**: pick a code repository already bound to the application, or select **Bind code repository** to bind one in place; it is selected automatically after binding. If no Git credential is available, connect an account under **Code Repositories → Git Credentials** first. Then choose the branch and Dockerfile, plus a registry with push credentials. Build first, then release the generated image.

Rarely used options live in the **Advanced (Kubernetes)** section. Items that have values expand automatically and show a count: workload and scaling behavior, container startup commands and probes, Service, config and secret files, deployment hooks, security context and scheduling, and auxiliary containers. JSON fields are validated before saving.

When autoscaling is enabled, the replica input is replaced by minimum replicas, maximum replicas, and CPU or memory target utilization. Set at least one target utilization.

Set replicas to `0` to retain the deployment configuration and its Kubernetes resources while stopping all Pods. Set replicas back to a positive integer to resume the workload. Switching between zero and a positive replica count applies directly to the current workload without creating a new Release. When autoscaling is enabled, the minimum replica count must be at least `1` because the current CPU and memory resource metrics cannot wake a workload from zero replicas.

Changes to the image, resource quotas, Service, volumes, or advanced runtime settings require a new Release to affect running instances. When the page offers **Save and redeploy**, review the impact before using it.

CPU and memory inputs are quotas, not Kubernetes requests or limits. Luna DevOps derives the actual fields from the selected runtime cluster policy; a `0%` policy value omits the corresponding field. A deployment cannot override its CPU or memory limit separately. The editor shows a floor-to-ceiling runtime cost estimate based on the selected cluster policy; see [Billing and Cost Analysis](../billing.md) for how runtime is billed.

Runtime metrics in the deployment details are labeled as totals across all replicas: CPU and memory usage is the live sum over every Pod and container (including sidecars), the denominator is the sum of the limits of the same containers, and the percentage is usage divided by that limit. If any container has no limit, the value shows `-`. The details also show the total request floor and the per-replica request / limit. These values are read from the current workload Pod template, so policy or quota changes appear only after a redeploy.

## Delete a deployment

Deleting a deployment checks service relations first. While an enabled service relation still targets it, deletion is rejected and neither its routes nor its runtime resources are removed; delete or disable those relations in the topology first. The check and the transition to **Deleting** happen in one transaction, and a deployment that is being deleted cannot become the target of a new service relation.

Deletion always ends as **Deleted** or **Delete failed**. When it fails, the deployment list shows the reason, such as a remaining service relation, a runtime cleanup failure, or a deletion task that could not be queued. Resolve the cause and delete the deployment again.
