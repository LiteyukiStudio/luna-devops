# App Marketplace

The App Marketplace provides installation templates for databases, caches, monitoring tools, and common self-hosted applications.

A template usually creates an application, an image-based deployment target, required configuration and volumes, and optionally the first release. Secret values are stored securely and are never shown again in the UI.

## Install a template

1. Open **App Marketplace** and find a template.
2. Select **Install**, then choose a project and runtime cluster.
3. Confirm the application name, image, resources, and storage.
4. Fill in required template settings. Leave supported generated passwords empty to let the platform create them.
5. Choose whether to deploy immediately and confirm.

The directory loads only summaries for search and comparison, then fetches the complete install parameters after you select a template. Secret parameters never include plaintext defaults, and a search with no match returns an empty result instead of failing.

After installation, the application deployment page opens. Check the release and workload status to confirm that the application is running.

## Platform components

Templates marked **Platform component** can only be installed by platform administrators. They add platform capabilities to a selected runtime cluster, such as gateway traffic collection, and remain managed by the platform.

Related pages identify whether a component is missing, waiting for valid data, or unavailable.

## Limitations

Templates are intended for applications that can start with their image's default command. Some applications still require an existing database, an external service, or additional initialization. Review the template notes and the application's official documentation before installation.

Installing separate applications does not automatically integrate them. For example, Prometheus and Grafana still require environment-specific scrape targets and data-source configuration.
