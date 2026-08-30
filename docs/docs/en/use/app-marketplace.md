# App Marketplace

App Marketplace provides curated templates grouped into seven categories: databases, storage, observability, developer tools, collaboration and content, middleware, and security tools.

## Install an application

1. Search or filter templates and read the purpose, prerequisites, and parameter notes.
2. Select **Install** and choose a project space and runtime cluster.
3. Review the application name, image, and resources, then select an existing volume or create a blank one in place.
4. Enter required parameters. Leave platform-generated passwords blank when supported.
5. Choose whether to deploy immediately and confirm installation.

The template creates an application and deployment and can create its first Release. Secret parameters are written only to the secret store and are never displayed in the page or installation result.

The Redis password is optional. A long, random password is recommended; leaving it blank runs Redis without password authentication. When provided, the password reaches the workload only through a Secret, and its plaintext is not written to ordinary environment settings or the installation-value snapshot.

After installation, check the Release, workload, logs, and access method. A template does not configure every third-party integration, such as Grafana data sources, application database accounts, or external OAuth.

The deployment stage must be `dev`, `test`, `staging`, or `prod`. The CLI exposes the stable `luna app-template install` command. Agent and CLI validation rejects values such as `default` or `qa` before sending the request and reports the allowed values. The platform fallback returns `deployment.stage_invalid`, the argument path, and `retryable: false`; automation must correct the argument instead of replaying it unchanged.

## Volumes

A template that declares persistent data requires a compatible volume from the same project space and cluster. The installation dialog can create a blank volume for the current runtime cluster with the mode required by the template and selects it automatically once creation is accepted. You can also select an existing compatible volume. If the creation task cannot be queued temporarily, the dialog keeps your input; retrying continues the same creation instead of producing a duplicate volume. The template never silently falls back to `emptyDir`. See [Runtime Data Persistence](./workflow/runtime-data-persistence) for other creation and connection methods.

## Platform components

Templates marked **Platform Component** are limited to platform administrators and add a capability to a selected runtime cluster, such as gateway traffic collection. They still run as ordinary applications, so their status and allowed configuration remain visible on the application and deployment pages.

> A template is an inspectable starting point, not a guarantee that every production environment is supported. Review the image version, architecture, resources, storage, and external dependencies before installation.
