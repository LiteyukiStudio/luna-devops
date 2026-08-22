# App Marketplace

App Marketplace provides curated templates grouped into seven categories: databases, storage, observability, developer tools, collaboration and content, middleware, and security tools.

## Install an application

1. Search or filter templates and read the purpose, prerequisites, and parameter notes.
2. Select **Install** and choose a project space and runtime cluster.
3. Review the application name, image, resources, and volumes.
4. Enter required parameters. Leave platform-generated passwords blank when supported.
5. Choose whether to deploy immediately and confirm installation.

The template creates an application and deployment and can create its first Release. Secret parameters are written only to the secret store and are never displayed in the page or installation result.

After installation, check the Release, workload, logs, and access method. A template does not configure every third-party integration, such as Grafana data sources, application database accounts, or external OAuth.

## Volumes

A template that declares persistent data requires a compatible volume from the same project space and cluster. It never silently falls back to `emptyDir`. Follow [Runtime Data Persistence](./workflow/runtime-data-persistence) to create or connect a volume first.

## Platform components

Templates marked **Platform Component** are limited to platform administrators and add a capability to a selected runtime cluster, such as gateway traffic collection. They still run as ordinary applications, so their status and allowed configuration remain visible on the application and deployment pages.

> A template is an inspectable starting point, not a guarantee that every production environment is supported. Review the image version, architecture, resources, storage, and external dependencies before installation.
