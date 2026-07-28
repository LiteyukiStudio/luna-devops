# App Marketplace

Use the app marketplace when you need a database, cache, or monitoring tool without configuring it from scratch. Built-in templates cover databases, caches, messaging, object storage, search, observability, registries, database administration, web servers, and common self-hosted apps, such as PostgreSQL, Redis, NATS, ClickHouse, Qdrant, Typesense, Garage, Prometheus, Grafana, Gitea, Vaultwarden, NocoDB, Wiki.js, and WordPress.

Template icons prefer official SVG or raster assets published by each application project. A traceable, properly licensed brand icon library is used only when an official reusable asset is unavailable, and the neutral placeholder is the final fallback. The platform does not draw approximate logos for established third-party products. External brand assets are vendored with the frontend, so the marketplace does not depend on an icon CDN at runtime.

The marketplace also includes a small set of platform components. Only platform administrators can install them. They live in the platform-owned `platform-system` project space and create normal applications, deployment targets, and Releases. This project space is visible only to platform administrators, cannot be deleted, and is excluded from user project billing. The built-in `Luna Gateway Traffic Probe` enables gateway traffic billing when needed.

Installing a template creates:

- An application, with the template icon applied by default. Application icons support preset icon names, site-local asset paths, or `http(s)` image URLs.
- An image-based deployment target.
- Template-defined environment variables, secret variables, and runtime data volumes.
- Template-defined config files and secret files; sensitive files are written into Kubernetes Secrets.
- An optional first Release; deployment is enabled by default.

The marketplace organizes discovery around search and overview, template filters, and a single template directory. All templates remain in the same directory. The category dropdown sits directly before popularity and sort-order controls as one list toolbar. Search covers application names, images, websites, and official repositories. Template cards show only the application icon, name, category, purpose, source links, and install action. Resource defaults and image references are confirmed in the install dialog.

The top hero uses a clean raised surface without gradients or ambient glows for search and template metrics, preserving stable separation from the global themed canvas in both light and dark modes.

Template cards provide official website and repository links so you can quickly verify the source before installing. Apps without a separate website use their official repository as the website link. The “Install” action opens the configuration dialog and does not create resources immediately.

Secret parameters are written to the platform secret store. Deployment targets keep secret references only, and plaintext values are not echoed back to the frontend.

## Install Flow

1. Open “App Marketplace”.
2. Pick a template and click “Install”.
3. Select a project space and confirm the application name, immutable identifier, runtime cluster, image reference, CPU, memory, replicas, and data capacity. The image reference is prefilled from the template and can be replaced with a Harbor, DockerHub proxy, or private registry image.
4. Fill in template parameters. Auto-generated passwords can be left empty; the backend generates them.
5. Keep “Deploy after install” enabled, or disable it and release manually from the application deployment page later.

After a successful install, the page navigates to the new application's deployment tab.

## Platform components

Platform components are still installed from “App Marketplace”, but they land in the platform-owned project space:

1. A platform administrator selects a template marked as “Platform Component”.
2. They choose the target runtime cluster.
3. They fill in the small set of component parameters, such as the Luna DevOps API URL.
4. The platform creates or reuses the component application under `platform-system`, creates a deployment target and Release for the selected runtime cluster, and generates a dedicated reporting token.
5. The worker deploys ConfigMap, Secret, Deployment, and Service through the normal application deployment path. The platform additionally ensures the probe ServiceAccount and read-only RBAC, so release logs, runtime logs, and Web Console are available from the application deployment page.

When Gateway Traffic Probe is not installed, the billing page shows gateway traffic as unavailable and guides platform administrators to install it from the marketplace. After installation, the probe sends a hello to the platform when it starts and refreshes the heartbeat before each scrape, so the billing page can show "deployed / waiting for report". Gateway traffic becomes available only after the probe successfully reports its first positive traffic window. This online state is stored as short-TTL runtime state in Redis or API process memory, not in the system component installation table, so manually deleted Kubernetes resources do not leave stale billing status behind.

Gateway Traffic Probe is published as the standalone `liteyukistudio/devops-gateway-traffic-probe` image. During installation, the platform injects `API_BASE_URL`, `REPORT_TOKEN`, `RUNTIME_CLUSTER_ID`, `TRAEFIK_METRICS_URL`, and related environment variables. The platform database stores only the report token hash, while the regular deployment Secret stores the plaintext token used for reporting. `TRAEFIK_METRICS_URL` is derived from the runtime cluster Gateway namespace by default, and it can be overridden in the install template with a cluster-reachable Traefik Prometheus metrics endpoint.

The template list supports category filtering, search by template name, image, website, or repository, and sorting by popularity weight or name.

## Usage Boundaries

Current templates are intended for images that can run with their default command. Prometheus currently ships with a minimal config that scrapes its own `/metrics`; Grafana and Prometheus remain independent single-app templates, so the platform does not auto-create Grafana data sources or discover application workloads. Garage is provided as a single-node lightweight object storage template; the platform generates its base config file. Multi-node layout initialization, bucket/key outputs, and richer connection details will be added with the later template outputs work. Templates such as WordPress and Mongo Express connect to an existing database, so prepare the database endpoint and credentials before installation.

Marketplace templates are loaded from backend-embedded JSON. Future third-party marketplaces can reuse the same schema, with backend-side fetching, validation, and caching.
