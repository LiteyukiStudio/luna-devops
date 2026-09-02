# Project Spaces and Applications

## Dashboard and project-space visibility

The dashboard is fixed to **Related to me**, aggregating only project spaces where the current account is a member and their related resources; it does not provide a platform-wide range switch. The project-space list also defaults to **Related to me**. Platform administrators switch that list to **All** only for a platform-wide inventory, cross-project troubleshooting, or an explicit search for another project space. **All** never bypasses backend authorization.

## Create a project space

A project space is the **logical isolation boundary** for a team and its resources. Split spaces by product, team, or customer. Members, applications, builds, releases, volumes, and routes are shared inside the space.

Enter a name and stable identifier, then add members with the minimum required roles. The identifier cannot be changed later.

## Choose member roles

The project-space role and the access credential's scopes are enforced together. An operation is allowed only when both permit it. Platform administrators sit outside the project-member role matrix and retain platform-wide administration, but must still satisfy the current credential's scopes.

- **Owner**: Manages the project space and its members, including project-space deletion and all project operations.
- **Admin**: Manages members and most project resources. Admins can delete applications, deployments, and routes, but cannot delete the project space.
- **Developer**: Creates, updates, releases, and restarts workloads. Developers cannot delete applications, deployments, routes, or project volumes.
- **Viewer**: Reads project resources and runtime status.

Among regular project members, only an Owner can delete a project space. Only Owners and Admins can delete applications, deployments, routes, and project volumes. Resource references, data impact, and in-progress work are still validated before deletion starts.

Build logs, live deployment metrics, AI task progress, and volume transfers continuously revalidate the account, credential scopes, and project role while connected. An active stream stops when the account is disabled, the session or token is revoked, the member is removed, or authorization becomes unavailable. Start a new request after access is restored.

## Create an application

An application is one independently deployable service, such as a Web frontend, API, or Worker. One repository can supply several applications, and one application can have development, test, staging, and production deployments.

1. Open **Applications** in the project space.
2. Create an application with a name, identifier, and description.
3. Open its **Deployments** page and create a deployment.

The application organizes the service. Configure its image, ports, resources, environment, and volumes in the deployment.

> Deleting an application blocks new delivery work and asynchronously cleans up platform-managed runtime resources. Project volumes are retained by default to prevent accidental data loss.
