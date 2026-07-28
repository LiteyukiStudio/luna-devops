# Deploy a Web Project

This is a complete walkthrough you can follow from start to finish. It uses [`snowykami/neo-blog`](https://github.com/snowykami/neo-blog) to build source code into images, release them to a runtime cluster, and create a public route for the frontend.

`neo-blog` has a Go backend at the repository root and a Next.js frontend under `web/`.

| Service | Directory | Dockerfile | Port | Notes |
| --- | --- | --- | ---: | --- |
| Frontend | `web/` | `web/Dockerfile` | `3000` | Public route points here. |
| Backend | repository root | `Dockerfile` | `8888` | Internal API service. |
| Data | backend data path | backend volume | - | Start with the backend data volume; move to PostgreSQL later if needed. |

Before you start, make sure the API and worker are running, a runtime cluster is configured, and a registry is available for build output. Missing any of these will block a later step.

## 1. Create a Project Space

Create a project space for the blog.

![Create project space](/guide/deploy-web-project/01-project-space.svg)

Recommended values:

| Field | Example |
| --- | --- |
| Name | `Neo Blog` |
| Identifier | `neo-blog` |
| Members | Start with yourself |

## 2. Create Applications

Create two applications in the project space.

![Create applications](/guide/deploy-web-project/02-applications.svg)

| Application | Identifier | Purpose |
| --- | --- | --- |
| `Neo Blog Frontend` | `neo-blog-frontend` | Public web UI. |
| `Neo Blog Backend` | `neo-blog-backend` | API and backend logic. |

For the first deployment, keep data with the backend data volume. Add an external database only when the app configuration needs it.

## 3. Bind the GitHub Repository

Bind this repository:

```text
snowykami/neo-blog
```

![Bind GitHub repository](/guide/deploy-web-project/03-repository.svg)

Use `main` as the default branch. The same repository binding can be reused by both frontend and backend deployment targets.

## 4. Create the Backend Deployment Target

Use these values:

| Field | Value |
| --- | --- |
| Source | Build from repository |
| Dockerfile | `Dockerfile` |
| Build context | `.` |
| Dockerfile Build Args | Optional, for example `EMBED_WEB=true` |
| Service port | `8888` |
| Image tag | `latest` or `${GIT_SHA}` |

Recommended environment variables:

| Variable | Example |
| --- | --- |
| `MODE` | `prod` |
| `PORT` | `8888` |
| `BASE_URL` | `https://blog.example.com` |
| `PASSWORD_SALT` | a stable random value |
| `JWT_SECRET` | a stable random value |

If you start with SQLite, mount a data volume at `/app/data`.

## 5. Create the Frontend Deployment Target

![Create deployment targets](/guide/deploy-web-project/04-deployment-targets.svg)

| Field | Value |
| --- | --- |
| Source | Build from repository |
| Dockerfile | `web/Dockerfile` |
| Build context | `web` |
| Dockerfile Build Args | Optional, for example `VERSION=${{ github.sha }}` |
| Service port | `3000` |
| Image tag | `latest` or `${GIT_SHA}` |

Dockerfile Build Args are passed to Dockerfile `ARG` instructions as BuildKit `build-arg` values and snapshotted into each build run. Use them for non-secret values such as `EMBED_WEB=true`, `VERSION=${{ github.sha }}`, or build modes. Put passwords, tokens, and private package credentials in project-space build secret variables instead.

Set `BACKEND_URL` to the backend service URL, for example:

```text
BACKEND_URL=http://neo-blog-backend:8888
```

Use the actual Service name shown by your platform deployment if it differs.

## 6. Build and Release

![Build and release](/guide/deploy-web-project/05-build-release.svg)

Recommended order:

1. Trigger the backend build.
2. Release the backend after the build succeeds.
3. Trigger the frontend build.
4. Release the frontend after the build succeeds.
5. Check that both workloads are healthy.

If a build fails, start from the build log tail. Common causes are base image pull failures, npm registry access, Go module download failures, or insufficient build memory.

## 7. Create the Public Route

Only expose the frontend.

![Create gateway route](/guide/deploy-web-project/06-gateway.svg)

| Field | Example |
| --- | --- |
| Application | `neo-blog-frontend` |
| Host | `blog.example.com` |
| Path | `/` |
| Service port | `3000` |

Open the route after it becomes healthy. If the page loads but API calls fail, check frontend `BACKEND_URL` and backend `BASE_URL`.

## 8. Enable Automation After the First Successful Run

Start manually. After the first successful deployment, enable webhook builds, auto release, branch filters, and tag filters one at a time. This makes failures easier to isolate and rollbacks easier to understand.
