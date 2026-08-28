# Project Spaces and Applications

## Choose the dashboard and project-space visibility

The dashboard and project-space list default to **Related to me**, aggregating only project spaces where the current account is a member and their related resources. This remains the default for platform administrators. Switch to **All** only for a platform-wide inventory, cross-project troubleshooting, or an explicit search for another project space. **All** never bypasses backend authorization.

## Create a project space

A project space is the **logical isolation boundary** for a team and its resources. Split spaces by product, team, or customer. Members, applications, builds, releases, volumes, and routes are shared inside the space.

Enter a name and stable identifier, then add members with the minimum required roles. The identifier cannot be changed later.

## Create an application

An application is one independently deployable service, such as a Web frontend, API, or Worker. One repository can supply several applications, and one application can have development, test, staging, and production deployments.

1. Open **Applications** in the project space.
2. Create an application with a name, identifier, and description.
3. Open its **Deployments** page and create a deployment.

The application organizes the service. Configure its image, ports, resources, environment, and volumes in the deployment.

> Deleting an application blocks new delivery work and asynchronously cleans up platform-managed runtime resources. Project volumes are retained by default to prevent accidental data loss.
