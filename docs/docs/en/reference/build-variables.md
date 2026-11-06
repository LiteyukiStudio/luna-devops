# Build Arguments and Build Variables

This page covers the two ways to pass values to a Dockerfile during a source build, their formats and precedence, and the platform environment variables available to build and deployment hooks.

| Method | Where to set it | Suitable for | Template variables |
| --- | --- | --- | --- |
| Build Args | Deploy config → Build → Build Args | Versions, switches, and other non-sensitive values | Supported |
| Build variables and secrets | Global settings, project space **Shared config → Build variables**, application, deploy config | Values shared by several apps; secrets | Not supported |

Both are passed to BuildKit as `build-arg`. A Dockerfile must declare `ARG NAME` before it can read a value.

## Build Args format

One `KEY=value` per line:

```text
# Lines starting with # and blank lines are ignored
EMBED_WEB=true
VERSION=${{ github.ref_name }}-{short_sha}
```

- If a line has no `=`, the first `:` is used as the separator, for example `MODE:prod`.
- You can also enter a JSON object such as `{"EMBED_WEB":"true"}`. Content that starts with `{` is parsed as JSON.
- Leading and trailing whitespace is trimmed from names and values. A later line overrides an earlier line with the same name.
- Names may contain letters, digits, and underscores, cannot start with a digit, and are at most 128 characters.
- Each value is at most 4096 bytes, and the whole field is at most 64 KiB.

Invalid input fails to save with error code `deployment.build_args_invalid`.

Build Args are snapshotted from the deploy config into every build record, where authorized members can see them. **Do not put passwords or tokens here**; use build secrets for sensitive values.

### Template variables in values

Build Arg values are rendered at build time with the same variables as image tags:

| Variable | Result |
| --- | --- |
| `${{ github.sha }}` | Full SHA of the checked-out commit |
| `{short_sha}` | First 12 characters of the commit SHA |
| `${{ github.ref_name }}` | Tag name when building a Git tag, otherwise the branch name |
| `${{ github.ref_type }}` | `tag` or `branch` |
| `${{ github.ref }}` | `refs/heads/<branch>` or `refs/tags/<tag>` |

Unlike image tags, Build Arg values are not character-normalized; `${{ github.ref }}` stays `refs/heads/main`. Other forms such as `{branch}` or `{commit}` are not replaced. See [Image Repository and Tag Templates](./image-templates#tag-variables) for details on each variable.

## Build variables and secrets

Build variables are merged in this order; later layers override earlier values with the same name:

1. Global build variables.
2. Build variable sets: project-space sets bound to the project space, in creation order, then the sets selected in the deploy config, in selection order.
3. Application build variables.
4. Deploy config build variables.

Rules:

- Names follow the Build Args rules; each value is at most 4096 bytes.
- At the winning layer, a name is either a plain variable or a secret, never both.
- Names starting with `LITEYUKI_` are reserved by the platform and ignored.
- When a Build Arg has the same name, the Build Arg value wins.
- Build variable values do not render template variables; `${{ github.sha }}` is passed through literally.
- Plain build variables are recorded in build records. Secret values are resolved only at build time, never returned, and masked in build logs.

> Secrets are also passed as `build-arg`. Build args used by `RUN` instructions may appear in the image build history. Do not reference secrets in the final runtime stage; prefer using them in an intermediate stage of a multi-stage build.

## Environment variables in hooks

Hook scripts are not template-rendered; read environment variables directly.

**Build hooks** (before/after repository pull, before/after build, before/after image push) run in the build container and can read:

| Variable | Meaning |
| --- | --- |
| `LITEYUKI_PROJECT_ID` | Project-space ID |
| `LITEYUKI_APPLICATION_ID` | Application ID |
| `LITEYUKI_DEPLOYMENT_TARGET_ID` | Deploy config ID |
| `LITEYUKI_BUILD_RUN_ID` | Build record ID |
| `LITEYUKI_BUILD_JOB_ID` | Build job ID |
| `LITEYUKI_HOOK_RUN_ID` | ID of this hook run |
| `LITEYUKI_HOOK_PHASE` | Hook phase, for example `preBuild` |
| `LITEYUKI_GIT_BRANCH` | Branch name; may be empty when building a tag |
| `LITEYUKI_GIT_TAG` | Git tag name; empty when building a branch |
| `LITEYUKI_GIT_REF_NAME` | Tag name or branch name |
| `LITEYUKI_GIT_REF_TYPE` | `tag` or `branch` |
| `LITEYUKI_GIT_REF` | `refs/heads/...` or `refs/tags/...` |
| `LITEYUKI_GIT_SHA` | Commit SHA; before the repository pull it is the requested commit and may be empty |
| `LITEYUKI_GIT_SHORT_SHA` | First 12 characters of the commit SHA |
| `LITEYUKI_IMAGE_REF` | Target image; from the before-build phase on, it is the final pushed image |

Build variables, secrets, and Build Args are also available to build hooks as environment variables with the same names.

**Deployment hooks** (before/after deployment) run as a separate Kubernetes Job using the released image and load the same plain and encrypted variables as the application container. They can read the variables above except `LITEYUKI_BUILD_JOB_ID`, plus:

| Variable | Meaning |
| --- | --- |
| `LITEYUKI_RELEASE_ID` | Release ID |
| `LUNA_DEPLOYMENT_STAGE` | Deployment stage |

In deployment hooks, `LITEYUKI_IMAGE_REF` is the released image. Git variables come from the build that produced the image and may be empty when an existing image is released directly.
