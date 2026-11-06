# App Marketplace

App Marketplace provides curated templates grouped into seven categories: databases, storage, observability, developer tools, collaboration and content, middleware, and security tools.

## Install an application

1. Search or filter templates and read the purpose, prerequisites, and parameter notes.
2. Select **Install** and choose a project space and runtime cluster.
3. Review the application name, image, and resources, then select an existing volume or create a blank one in place.
4. Enter required parameters. Leave platform-generated passwords blank when supported.
5. Choose whether to deploy immediately and confirm installation.

The template creates an application and deployment and can create its first Release. A multi-component template creates one set per component; see [Multi-component templates](#multi-component-templates). Secret parameters are written only to the secret store and are never displayed in the page or installation result.

The Redis password is optional. A long, random password is recommended; leaving it blank runs Redis without password authentication. When provided, the password reaches the workload only through a Secret, and its plaintext is not written to ordinary environment settings or the installation-value snapshot.

After installation, check the Release, workload, logs, and access method. A template does not configure every third-party integration, such as Grafana data sources, application database accounts, or external OAuth.

The deployment stage must be `dev`, `test`, `staging`, or `prod`. The CLI exposes the stable `luna app-template install` command. Agent and CLI validation rejects values such as `default` or `qa` before sending the request and reports the allowed values. The platform fallback returns `deployment.stage_invalid`, the argument path, and `retryable: false`; automation must correct the argument instead of replaying it unchanged.

## Volumes

A template that declares persistent data requires a compatible volume from the same project space and cluster. The installation dialog can create a blank volume for the current runtime cluster with the mode required by the template and selects it automatically once creation is accepted. You can also select an existing compatible volume. If the creation task cannot be queued temporarily, the dialog keeps your input; retrying continues the same creation instead of producing a duplicate volume. The template never silently falls back to `emptyDir`. See [Runtime Data Persistence](./workflow/runtime-data-persistence) for other creation and connection methods.

## Multi-component templates

Some templates consist of several components. For example, Neo Blog includes a frontend, a backend, PostgreSQL, and Redis. The installation dialog lists each component's image and default resources. Installation creates one application and deployment per component, with the identifier `<application identifier>-<component>`, such as `blog-web` and `blog-api`. Leave room in the application identifier for the longest component suffix; the dialog shows every identifier that will be created.

- Components reach each other through service names in the same project space. The platform fills in these addresses during installation.
- Every dependency between components is recorded as a service relation (for example `blog-api` → `blog-postgres`). It appears in the project topology and also injects `<COMPONENT>_HOST` and `<COMPONENT>_PORT` environment variables, such as `POSTGRES_HOST`. A dependency cannot be deleted on its own while a dependant still exists; to remove the whole installation, delete the dependant applications before the database and other dependencies.
- Shared passwords, such as database and Redis passwords, are generated or entered once and injected as Secrets into both the dependencies and the main application. Their plaintext never appears in the page, installation result, or value snapshot.
- Each component that declares persistent data needs its own volume. One volume cannot be selected for several components.
- Multi-component templates install with each component's image and resources. You cannot override the image, replicas, CPU, or memory for all components at install time; adjust each component's deployment afterward.

After installation, each component appears as an application in the application list. With **Deploy after install**, every component gets its first Release at the same time, and installation progress is derived from those Releases: it succeeds only when all of them succeed and fails as soon as one fails. The main application may restart a few times until its dependencies are ready. If it keeps failing, check that component's Release and logs.

### Neo Blog

- Requires two filesystem volumes: one for backend uploads and one for the PostgreSQL database. Redis only holds shared state and uses temporary storage.
- **Site URL** is the public address of the blog. You can leave it blank and set it later in the blog console. The password salt and JWT secret are generated when left blank; do not change the password salt after users sign up.
- After installation, [create an access route](./workflow/routes) to the frontend component (`<application identifier>-web`, port 3000). The backend does not need to be exposed.

## Platform components

Templates marked **Platform Component** are limited to platform administrators and add a capability to a selected runtime cluster, such as gateway traffic collection. They still run as ordinary applications, so their status and allowed configuration remain visible on the application and deployment pages.

## Troubleshooting installation

- **A required value is missing**: fill in the indicated template parameter and install again.
- **Conflicts with an existing resource**: the application identifier is usually taken or still being deleted. Retry with another identifier.
- **Installation failed**: installation runs in a single transaction, so a failure leaves no partial applications, deployments, or secrets and can be retried. If it keeps failing, or the template catalog or component dependencies are reported invalid, contact a platform administrator.

> A template is an inspectable starting point, not a guarantee that every production environment is supported. Review the image version, architecture, resources, storage, and external dependencies before installation.
