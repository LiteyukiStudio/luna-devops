# Platform Startup Problems

This page covers the Docker Compose problems most likely to block the platform from starting. For application builds or Kubernetes runtime failures, continue with the troubleshooting guide under Use.

## Run a specific image tag

The default `docker-compose.yaml` uses the `nightly` images. To verify an RC or stable release, set `DEVOPS_IMAGE_TAG` before starting:

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

If you want to build images from the current source tree instead of pulling DockerHub images, use the source-build Compose file:

```bash
docker compose -f docker-compose-build.yaml up -d --build
```

## Port `8088` is occupied

Check the listener:

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

You can stop the existing process or change the port mapping in `docker-compose.yaml`:

```yaml
ports:
  - "8089:8080"
```

Then visit `http://localhost:8089`.

## The page opens, but API calls fail

Check API logs:

```bash
docker compose logs -f api
```

Then confirm PostgreSQL and Redis are healthy:

```bash
docker compose ps
```

## Worker did not start

Check worker logs:

```bash
docker compose logs -f worker
```

The Worker handles builds, deployments, and status synchronization. The API alone is enough to browse the console, but the Worker must stay healthy before you can build or release an application.

## Use platform build templates

Deployment targets support two build definition modes:

- **Repository Dockerfile** uses a Dockerfile already maintained in the repository.
- **Platform build template** generates the Dockerfile used by the current build from a small set of parameters.

The platform includes templates for:

- Go services
- Node.js services, Node.js static sites, Next.js services, and Bun services
- Python services using uv
- Rust services
- Ruby services
- Java services using Maven or Gradle
- .NET services
- Plain static sites

The template picker uses official project marks bundled with the console, so opening it does not contact a third-party icon service.

You can adjust dependency installation, build and start commands, service ports, and other required parameters, then preview the generated Dockerfile before saving. Java templates use JDK/JRE 21 by default, while the .NET template uses .NET 8. Adjust the parameters when your project uses a different version or artifact name.

Templates never modify the repository. The Worker mounts the generated Dockerfile as a separate file in the Kubernetes build Job and asks BuildKit to use it with the original repository build context. When a platform template is selected, it overrides a Dockerfile that may already exist in the repository.

Each build snapshots the template ID, immutable template version, parameter values, rendered Dockerfile checksum, and an internal copy of the rendered Dockerfile. Later deployment-target changes do not alter historical build records.

### Choose a template

1. Create or edit a deployment target from the application's Deploy page.
2. Select a repository. The platform inspects its files and puts likely templates first.
3. Select **Platform build template** under **Build definition**.
4. Choose a template, review its parameters, and preview the Dockerfile.
5. Save the deployment target and create a build.

Recommendations only use files such as `package.json`, `bun.lock`, `pyproject.toml`, `go.mod`, `Cargo.toml`, `Gemfile`, `pom.xml`, `build.gradle`, `*.csproj`, and `index.html`. The platform does not guess project-specific start commands; verify them against the project's documentation.

### Next.js service template

The Next.js service template follows the official Next.js Docker standalone example and separates dependency installation, compilation, and runtime into three stages. It selects npm, pnpm, or Yarn from `package-lock.json`, `pnpm-lock.yaml`, or `yarn.lock`. The runtime image only receives the standalone output, `public`, and `.next/static`, then starts `server.js` as a non-root user.

Enable standalone output in `next.config.js`, `next.config.mjs`, or `next.config.ts` before using the template:

```ts
const nextConfig = {
  output: 'standalone',
}

export default nextConfig
```

The platform does not rewrite repository configuration. When standalone output is missing, the build fails with an explicit message instead of producing an image that cannot start. Repositories containing `next.config.js`, `next.config.mjs`, or `next.config.ts` receive the Next.js service recommendation first.

The platform Gateway API access entry remains the reverse proxy in production. Before running multiple Next.js replicas, review shared ISR/data caches, cache-tag coordination, `NEXT_SERVER_ACTIONS_ENCRYPTION_KEY`, and version skew so replicas do not serve inconsistent results. See the official [Next.js deployment guide](https://nextjs.org/docs/app/getting-started/deploying), [standalone output reference](https://nextjs.org/docs/app/api-reference/config/next-config-js/output), and [self-hosting guide](https://nextjs.org/docs/app/guides/self-hosting).
