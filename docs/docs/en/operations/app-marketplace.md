# App Marketplace

The app marketplace installs common infrastructure apps into project spaces from templates. The MVP includes Redis, Valkey, Memcached, PostgreSQL, MySQL, MariaDB, MongoDB, RabbitMQ, Garage, Prometheus, Grafana, Uptime Kuma, Memos, IT-Tools, Excalidraw, Verdaccio, Docker Registry, pgAdmin4, Meilisearch, and Bytebase for quick cache, database, queue, object storage, observability, and small-team tooling setup.

Installing a template creates:

- An application, with the template icon applied by default. Application icons support preset icon names, site-local asset paths, or `http(s)` image URLs.
- An image-based deployment target.
- Template-defined environment variables, secret variables, and runtime data volumes.
- Template-defined config files and secret files; sensitive files are written into Kubernetes Secrets.
- An optional first Release; deployment is enabled by default.

Template cards show the image, official website, and official repository together so you can quickly verify the source before installing. Apps without a separate website use their official repository as the website link.

Secret parameters are written to the platform secret store. Deployment targets keep secret references only, and plaintext values are not echoed back to the frontend.

## Install Flow

1. Open “App Marketplace”.
2. Pick a template and click “Install”.
3. Select a project space and confirm the application name, slug, runtime cluster, image reference, CPU, memory, replicas, and data capacity. The image reference is prefilled from the template and can be replaced with a Harbor, DockerHub proxy, or private registry image.
4. Fill in template parameters. Auto-generated passwords can be left empty; the backend generates them.
5. Keep “Deploy after install” enabled, or disable it and release manually from the application deployment page later.

After a successful install, the page navigates to the new application's deployment tab.

The template list supports category filtering, search by template name, image, website, or repository, and sorting by popularity weight or name. Built-in templates intentionally skip PHP applications such as Adminer and phpMyAdmin for now.

## Current Limits

The MVP only enables templates whose images can run with their default command. Prometheus currently ships with a minimal config that scrapes its own `/metrics`; Grafana and Prometheus remain independent single-app templates, so the platform does not auto-create Grafana data sources or discover application workloads. Garage is provided as a single-node lightweight object storage template; the platform generates its base config file. Multi-node layout initialization, bucket/key outputs, and richer connection details will be added with the later template outputs work.

Marketplace templates are loaded from backend-embedded JSON. Future third-party marketplaces can reuse the same schema, with backend-side fetching, validation, and caching.
