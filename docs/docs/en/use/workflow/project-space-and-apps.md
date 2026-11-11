# Project Spaces and Applications

## Dashboard and project-space visibility

The dashboard is fixed to **Related to me**, aggregating only project spaces where the current account is a member and their related resources; it does not provide a platform-wide range switch. The project-space list also defaults to **Related to me**. Platform administrators switch that list to **All** only for a platform-wide inventory, cross-project troubleshooting, or an explicit search for another project space. **All** never bypasses backend authorization.

Application counts on the dashboard and the project-space overview exclude applications that are being deleted or were deleted. The latest activity in the dashboard's recent applications is each application's own most recent event, and the latest activity of a project-space shortcut is that project space's own most recent event. The failed release count on the project-space overview only includes releases that failed in the last 7 days.

The **Access routes** metric on the project-space overview shows ready routes out of enabled routes. The gateway is observed every time the overview opens; disabled routes and routes being deleted are not counted. If any route cannot be observed, the metric shows that live gateway status is unavailable instead of zero ready routes. Confirm that the runtime cluster and gateway are reachable, then refresh the page.

## Create a project space

A project space is the **logical isolation boundary** for a team and its resources. Split spaces by product, team, or customer. Members, applications, builds, releases, volumes, and access routes are shared inside the space.

Enter a name and stable identifier, then add members with the minimum required roles. The identifier cannot be changed later.

A project space opens on **Overview**. The sidebar lists **Overview**, **Applications**, **Shared config**, **Volumes**, **Members**, and **Service relations**. Build variables, runtime config, and hooks shared by several applications live under **Shared config**, one tab each.

## Choose member roles

The project-space role and the access credential's scopes are enforced together. An operation is allowed only when both permit it. Platform administrators sit outside the project-member role matrix and retain platform-wide administration, but must still satisfy the current credential's scopes.

- **Owner**: Manages the project space and its members, including project-space deletion and all project operations.
- **Admin**: Manages members and most project resources. Admins can delete applications, deployments, and access routes, but cannot delete the project space.
- **Developer**: Creates, updates, releases, and restarts workloads. Developers cannot delete applications, deployments, access routes, or project volumes.
- **Viewer**: Reads project resources and runtime status.

Among regular project members, only an Owner can delete a project space. Only Owners and Admins can delete applications, deployments, access routes, and project volumes. Resource references, data impact, and in-progress work are still validated before deletion starts.

Build logs, live deployment metrics, AI task progress, and volume transfers continuously revalidate the account, credential scopes, and project role while connected. An active stream stops when the account is disabled, the session or token is revoked, the member is removed, or authorization becomes unavailable. Start a new request after access is restored.

## Create an application

An application is one independently deployable service, such as a Web frontend, API, or Worker. One repository can supply several applications, and one application can have development, test, staging, and production deployments.

1. Open **Applications** in the project space.
2. Create an application with a name, identifier, and description.
3. Open its **Deployments** page and create a deployment.

The application organizes the service. Configure its code repository, image, ports, resources, environment, and volumes in the deployment. The application detail follows the delivery order: **Overview**, **Builds**, **Deployments**, **Access routes**, **Topology**, and **Settings**. **Settings** only edits the application's basic information and lets you view, maintain, or unbind bound code repositories.

> Deleting an application blocks new delivery work and asynchronously cleans up platform-managed runtime resources. Project volumes are retained by default to prevent accidental data loss.

If another application still references this application through a service relation, deletion is rejected before any runtime resource is removed; delete or disable those relations in the topology first. Relations between deployment targets of the same application do not block deletion. An application that is being deleted cannot become the target of a new service relation.

Deletion always ends as **Deleted** or **Delete failed**. When it fails, the application list shows the reason, such as a remaining service relation, a runtime cleanup failure, or a deletion task that could not be queued. Resolve the cause and delete the application again.

## Configure network isolation

By default, applications in a project space can reach each other, the gateway can reach applications, and applications can reach the public internet. Other project spaces, private and reserved addresses, cluster nodes, and the Kubernetes API are always unreachable. When you need stricter isolation, a project space admin can tighten it under **Topology → Network isolation**; saving reapplies the network policies immediately.

| Setting | Value | Effect |
| --- | --- | --- |
| Ingress | Project-wide (default) | Applications in the project space can reach each other |
|  | Service bindings only | Each deployment target accepts only its own replicas and deployment targets that bind to it in the topology; applications without a service binding cannot reach each other |
| Egress | Internet (default) | The public internet and other applications' routes are reachable |
|  | Allowlist only | Only the addresses in the egress allowlist and routes are reachable |
|  | None | Only applications in the project space and cluster DNS are reachable |

The egress allowlist takes one `CIDR port,port` entry per line, at most 64 entries, for example `1.1.1.1/32 443`. Listed ports are TCP only; omitting ports allows all ports and protocols, so leave ports out when you need UDP such as external DNS or NTP. Private and reserved ranges are always denied and cannot be listed; contact a platform administrator when you need to reach an internal service.

Platform administrators can grant egress exceptions that allow specific internal addresses; granted exceptions are listed under the **Network isolation** card and apply in every egress mode.

Before switching to **Service bindings only**, make sure every call between applications has a service binding in the topology; undeclared calls fail immediately.

### Cross-project sharing

When another project space needs to reach a service in this project space directly, such as a shared database or an authentication service, use **Topology → Cross-project sharing**. Sharing only opens the network; credentials are not passed, so provide connection credentials separately.

1. A provider project space admin clicks **Share service**, chooses a deployment target, and enters the consumer's project space identifier (not its name).
2. A consumer project space admin clicks **Accept** under **Services shared with this project space**.
3. When the status is **Active**, consumer applications reach the service in the cluster at the address shown in the list (`<service>.<namespace>.svc`).

A share works in every ingress and egress mode on both sides. It allows consumer applications and deployment hooks, not builds, and reaches only the application Pods of the provider's deployment target. A share applies on the cluster where the provider's deployment target runs; the consumer must be able to use that cluster and deploy there, and the list shows the cluster name. A deployment target can be shared with at most 50 project spaces, and a project space can have at most 20 shares that are not yet accepted; the consumer's name is not shown until it accepts.

When the provider holds egress exceptions granted by the platform on the target's cluster, the status is **Awaiting maintainer approval** until that cluster's maintainer approves it. This is evaluated continuously: moving the deployment target to another cluster, or the provider receiving an exception later, requires approval again from that cluster's maintainer, and the share pauses until then. After the provider revokes the share or the consumer leaves it, the platform reapplies both sides' policies immediately and access usually stops within seconds; deleting a project space, application, or deployment target removes its shares. Creating, accepting, approving, and revoking are audited.

### Internal services

For deployment targets that do not serve external traffic, such as databases, queues, or background workers, enable **Internal service** under the deployment target's **Advanced → Service**. Gateway traffic then cannot reach its Pods and it cannot have routes; delete existing routes first. This requires the gateway controller to run on the Pod network; with hostNetwork, gateway traffic arrives from node addresses and cannot be told apart.
