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

A repository path on a registry belongs to one project space. If deploy configs or builds of another project space already used it, saving a deploy config returns `deployment_target.image_repository_claimed` and starting a build returns `build.image_repository_claimed`. Applications and deploy configs in the same project space can share a repository.

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
| `{commit}` | Same as `${{ github.sha }}` | `1234567890abcdef1234567890abcdef12345678` |
| `{shortSha}` | Same as `{short_sha}` | `1234567890ab` |
| `{branch}` | Branch name; empty when building a Git tag | `feature/login` |
| `{branchSlug}` | Branch name as lowercase letters, digits, and `-` | `feature-login` |
| `{tag}` | Git tag name; empty when building a branch | `v1.2.0` |
| `{tagSlug}` | Tag name as lowercase letters, digits, and `-` | `v1-2-0` |

These forms are case-sensitive, and spaces must match the table exactly. Forms not listed here (for example `{sha}` or `{ref_name}`) are not replaced.

The rendered tag is then normalized: each run of characters outside `A-Z a-z 0-9 _ . -` becomes one `-`, leading and trailing `.` and `-` are removed, and the result is cut to 128 characters. An empty result becomes `latest`.

`buildcache` is reserved for the platform build cache: a build whose tag renders to `buildcache` fails before pushing, and a `buildcache` tag in the registry is not offered as a release image.

**Example**: branch `feature/login`, commit `1234567890abcdef…`:

| Target image | Pushed image |
| --- | --- |
| `team/api:${{ github.ref_name }}-{short_sha}` | `team/api:feature-login-1234567890ab` |
| `team/api:${{ github.ref }}` | `team/api:refs-heads-feature-login` |
| `team/api:{short_sha}` | `team/api:1234567890ab` |
| `team/api:{branchSlug}-{shortSha}` | `team/api:feature-login-1234567890ab` |

### Tag template in a push credential

A credential's **Image tag template** generates the default tag for new deploy configs. When a deploy config is created, the [repository path template variables](#repository-path-template-variables) are replaced and the build-time variables above are kept as is, to be rendered on every build. For example, the template `{stage}-{shortSha}` produces the default tag `prod-{shortSha}`, and a build pushes `prod-1234567890ab`. If any other unknown placeholder remains, the default tag falls back to `latest`.

## Common mistakes

- **The pushed tag is a literal such as `sha` or `ref-name`**: a form not listed in the table was used. It is not replaced at build time and is only normalized into a literal. Use the forms in the table above.
- **The repository path contains `{...}`**: the repository template has a typo or an unknown variable. Unknown variables are kept as is and the push fails. Fix the variable name, save the credential, and restore the default target image in the deploy config.
- **DockerHub rejects the push**: the first path segment is not a user or organization you can push to. Change the template to `your-user/{projectIdentifier}-{appIdentifier}`.
- **`{target}` renders as `app`**: the deploy config name contains only non-ASCII characters or symbols, so normalization leaves nothing. Use `{stage}`, or include letters or digits in the name.
- **Saving a deploy config reports that the repository is used by another project space**: choose a different repository path, for example one that includes this project space identifier. To share an image, build it in the project space that already owns the repository.
- **The build log reports `image tag "buildcache" is reserved`**: the rendered tag equals the reserved cache tag, for example `{branch}` used as the tag while building a branch named `buildcache`. Change the tag template and retry.
- **The image shown during a build differs from the final one**: before a build finishes the platform shows an estimate. After success, the actually pushed image is authoritative.
