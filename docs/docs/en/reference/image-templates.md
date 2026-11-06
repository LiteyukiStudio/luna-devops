# Image Repository and Tag Templates

An image pushed by a source build has three parts: `<registry host>/<repository path>:<tag>`. This page lists the variables available in repository path templates and tag templates, how they render, and their limits.

## Where to set them

| Location | Field | Purpose |
| --- | --- | --- |
| Registries → push credential | Image repository template | Generates the default repository path for new deploy configs. |
| Registries → push credential | Image tag template | Generates the default tag for new deploy configs. Default: `latest`. |
| Deploy config → Basics and source | Target image | The `repository:tag` used by every build. You can edit it directly. |
| Application → Builds → manual build | Target image | Applies to that build only. |

Credential templates apply only while a deploy config's target image is empty or still the platform default. A target image you edited is kept as is. Changing a credential template does not rewrite existing deploy configs.

## Repository path template variables

Default template: `{registryNamespace}/{projectIdentifier}-{appIdentifier}`. Variables can be written as `{name}` or `${name}`; both forms are equivalent.

| Variable | Meaning | Source |
| --- | --- | --- |
| `{registryNamespace}` | Registry namespace | The project-space identifier when the registry has no namespace (true for every registry created in the console). |
| `{projectIdentifier}` | Project-space identifier | The project-space identifier, or its name when the identifier is empty. |
| `{project}` | Same as `{projectIdentifier}` | Alias. |
| `{appIdentifier}` | Application identifier | The application identifier, or its name when the identifier is empty. |
| `{app}` | Same as `{appIdentifier}` | Alias. |
| `{applicationIdentifier}` | Same as `{appIdentifier}` | Alias. |
| `{application}` | Same as `{appIdentifier}` | Alias. |
| `{stage}` | Deployment stage | `dev`, `test`, `staging`, or `prod`. |
| `{target}` | Deploy config name | The deploy config display name, or the stage when the name is empty. |

Except for `{registryNamespace}`, every value is normalized: lowercased, each run of characters outside `a-z0-9` replaced with one `-`, and leading or trailing `-` removed. An empty result becomes `app`. After rendering, leading and trailing `/` are removed and repeated `/` are merged.

**Example**: project-space identifier `neo-blog`, application identifier `frontend`, stage `prod`, deploy config name `Prod Web`:

| Template | Result |
| --- | --- |
| `{registryNamespace}/{projectIdentifier}-{appIdentifier}` | `neo-blog/neo-blog-frontend` |
| `team/{project}-{app}-{stage}` | `team/neo-blog-frontend-prod` |
| `team/{app}/{target}` | `team/frontend/prod-web` |

### Registry host prefix

- If the first path segment contains `.` or `:`, or is `localhost`, the path already includes a host and is used as is, for example `harbor.example.com/team/app`.
- DockerHub registries get no host prefix. The first path segment must be your DockerHub user or organization name.
- Other registries get the host from the registry endpoint, so `https://harbor.example.com` plus `team/app` becomes `harbor.example.com/team/app`.

## Tag variables

The tag part of the deploy config or manual build **Target image** is rendered at build time. These variables are available:

| Variable | Meaning | Example value |
| --- | --- | --- |
| `${{ github.sha }}` | Full SHA of the checked-out commit | `1234567890abcdef1234567890abcdef12345678` |
| `{short_sha}` | First 12 characters of the commit SHA | `1234567890ab` |
| `${{ github.ref_name }}` | Tag name when building a Git tag, otherwise the branch name | `main`, `v1.2.0` |
| `${{ github.ref_type }}` | `tag` or `branch` | `branch` |
| `${{ github.ref }}` | Full ref | `refs/heads/main`, `refs/tags/v1.2.0` |

These forms are case-sensitive, and spaces must match the table exactly.

The rendered tag is then normalized: each run of characters outside `A-Z a-z 0-9 _ . -` becomes one `-`, leading and trailing `.` and `-` are removed, and the result is cut to 128 characters. An empty result becomes `latest`.

**Example**: branch `feature/login`, commit `1234567890abcdef…`:

| Target image | Pushed image |
| --- | --- |
| `team/api:${{ github.ref_name }}-{short_sha}` | `team/api:feature-login-1234567890ab` |
| `team/api:${{ github.ref }}` | `team/api:refs-heads-feature-login` |
| `team/api:{short_sha}` | `team/api:1234567890ab` |

### Tag template in a push credential

A credential's **Image tag template** only generates the default tag for new deploy configs, and only the [repository path template variables](#repository-path-template-variables) are replaced in it. If any other placeholder remains, including the build-time variables above, the default tag falls back to `latest`. To tag by commit or branch, write the build-time variables directly in the deploy config **Target image**, for example `team/api:{short_sha}`.

## Common mistakes

- **The pushed tag is a literal such as `branch` or `shortSha`**: forms like `{branch}`, `{shortSha}`, `{commit}`, `{branchSlug}`, or `{tag}` are not replaced at build time; they are only normalized into literals. Use the forms in the table above.
- **The repository path contains `{...}`**: the repository template has a typo or an unknown variable. Unknown variables are kept as is and the push fails. Fix the variable name, save the credential, and restore the default target image in the deploy config.
- **DockerHub rejects the push**: the first path segment is not a user or organization you can push to. Change the template to `your-user/{projectIdentifier}-{appIdentifier}`.
- **`{target}` renders as `app`**: the deploy config name contains only non-ASCII characters or symbols, so normalization leaves nothing. Use `{stage}`, or include letters or digits in the name.
- **The image shown during a build differs from the final one**: before a build finishes the platform shows an estimate. After success, the actually pushed image is authoritative.
