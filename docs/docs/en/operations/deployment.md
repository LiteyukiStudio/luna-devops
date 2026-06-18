# Feature Map

Liteyuki DevOps connects code, images, clusters, and routes into one delivery path. You do not need to understand every underlying component before using it.

## Project spaces

A project space is the boundary for teams and resources. Members, environments, applications, build records, releases, and routes all belong to a project space.

Common patterns:

- One product per project space.
- One small team per project space.
- One customer or demo environment per project space.

## Applications

An application is a deployable service. One repository can map to multiple applications, such as API, Web, and Worker in a monorepo.

The application stores basic service information. Build source, image, environment variables, and release policy live in deployment targets.

## Deployment targets

A deployment target answers how an application should ship:

- Build from a repository or use an existing image.
- Publish to which environment.
- Use which registry.
- Listen on which service port.
- Auto release after a successful build or not.

When build variables or runtime config sets are deleted, the platform removes those references from deployment targets that still point to them, so deployment targets do not keep stale configuration IDs.

Gateway routes are enabled by default when created. To temporarily stop public access without losing the domain config, disable the route; the platform keeps the config and removes the runtime Ingress, then reapplies it when enabled again.

## Builds and releases

Builds create images. Releases deploy images to runtime clusters.

For a first run, deploy an existing image before wiring Git providers and automated builds.

## Routes

Routes connect domain, path, TLS, and backend service. After creating one, the platform shows apply status and checks so you can verify the service is reachable.
