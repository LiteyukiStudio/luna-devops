# Platform Startup Problems

This page covers common Docker Compose startup problems. For application build issues, see [FAQ](/en/start/faq).

## Run a specific version

```bash
DEVOPS_IMAGE_TAG=v0.1.0-rc.1 docker compose up -d
```

The default is `nightly`. Use a published fixed version in production.

## Port `8088` is occupied

Find the process:

```bash
lsof -nP -iTCP:8088 -sTCP:LISTEN
```

Stop that process, or change the port mapping in `docker-compose.yaml` to an available port such as `8089:8080`, then open the new port.

## The page opens, but API calls fail

```bash
docker compose ps
docker compose logs -f api
```

Confirm that API, PostgreSQL, and Redis are healthy. If the log reports authentication or connection errors, check the database and Redis settings in `.env`.

## Tasks never start

```bash
docker compose logs -f worker
```

Builds and releases require a healthy Worker. Fix the connection or configuration error in the log, then restart the affected service.

If the problem remains, collect the version, `docker compose ps` output, and relevant service logs before opening an issue. Remove tokens, passwords, and credentials embedded in connection strings first.
