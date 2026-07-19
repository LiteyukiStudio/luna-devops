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
- Node.js services, Node.js static sites, and Bun services
- Python services using uv
- Rust services
- Ruby services
- Java services using Maven or Gradle
- .NET services
- Plain static sites

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
