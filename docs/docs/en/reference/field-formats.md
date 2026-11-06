# Deploy Config Field Formats and Ranges

This page lists deploy config fields with format constraints. For what each field does and the steps, see [Create a Deploy Config](../use/workflow/deployment). OpenAPI is the authoritative source for the full API fields.

## Basics

| Field | Allowed values / format | Default | Notes |
| --- | --- | --- | --- |
| Stage | `dev`, `test`, `staging`, `prod` | `dev` | Cannot be changed after creation. `production` is normalized to `prod`. Other values return `deployment.stage_invalid`. |
| Name | Up to 120 characters; any language and symbols | The stage | Display only. It is also the source of `{target}` in image templates. |
| Workload type | `Deployment`, `StatefulSet` | `Deployment` | |

## Resources and replicas

| Field | Allowed values / format | Default |
| --- | --- | --- |
| CPU quota per replica | Positive Kubernetes quantity, for example `250m`, `0.5`, `1`, `2` | `1` |
| Memory quota per replica | Positive Kubernetes quantity, for example `512Mi`, `1Gi` | `1Gi` |
| Build CPU | Same as CPU quota | `2` |
| Build memory | Same as memory quota | `4Gi` |
| Replicas | Integer `≥ 0`; `0` stops every Pod | `1` |
| Autoscaling min replicas | Integer `≥ 1` | `1` |
| Autoscaling max replicas | Integer `≥` min replicas | Equal to min replicas |
| CPU / memory target utilization | Integer `0`–`1000` (percent); `0` disables that metric; with autoscaling on, at least one must be above `0` | `0` |
| Build timeout | Minutes in the console; seconds in the API, `60`–`86400` | 30 minutes |

An invalid or non-positive resource quantity returns `deployment.resource_quantity_invalid`. `Mi` and `Gi` are binary units, `M` and `G` are decimal; `1G` is smaller than `1Gi`. CPU and memory are quotas; the runtime cluster's resource allocation policy computes the actual requests and limits.

## Service ports

- Up to 16 ports per deploy config; port numbers `1`–`65535`; no duplicates.
- The first port is the default port. Add more ports when you need to expose extras such as metrics.
- Port names are lowercased, underscores and spaces become `-`, other symbols are removed, the result is at most 63 characters, and names must be unique. An empty name becomes `http` for the first port and `port-<number>` for the others.

## Auto-release trigger rules

Branch and tag matching use the same syntax:

- Empty matches everything. Separate multiple rules with commas, for example `main,release-*`.
- `*` matches any characters except `/`, and `?` matches one character. A lone `*` matches everything.
- `release-*` does not match `release/1.0`; write `release/*` instead.

## Container startup

| Field | Format |
| --- | --- |
| Command, arguments | JSON string array such as `["sh","-c","echo ok"]`, or one argument per line. |
| Dropped Linux capabilities | JSON string array or one per line, for example `ALL`. |
| Image pull policy | `Always`, `IfNotPresent`, `Never`; empty lets the platform decide per release action. |

The non-JSON form is split on both newlines and commas. If an argument contains a comma, use a JSON array. Invalid JSON makes the release fail.

## Kubernetes JSON fields

These fields take the JSON of the matching Kubernetes object, and the structure is validated on save:

| Field | JSON type |
| --- | --- |
| Readiness / liveness / startup probe | `Probe` object |
| Lifecycle | `Lifecycle` object |
| Tolerations | `Toleration` array |
| Affinity | `Affinity` object |
| Topology spread constraints | `TopologySpreadConstraint` array |
| Autoscaling behavior | `HorizontalPodAutoscalerBehavior` object |
| Init containers, sidecar containers | `Container` array |

Readiness probe example:

```json
{"httpGet":{"path":"/healthz","port":8080},"initialDelaySeconds":5,"periodSeconds":10}
```

## Key-value fields

Node selector and Service annotations accept either:

- a JSON object, for example `{"kubernetes.io/arch":"amd64"}`; or
- one `KEY=VALUE` per line. Blank lines and lines starting with `#` are ignored.

## Security context

| Field | Allowed values |
| --- | --- |
| Run-as UID, run-as GID, filesystem group GID | Non-negative integer; empty uses the image default |
| Filesystem group change policy | `Always`, `OnRootMismatch` |
| Service session affinity | `None`, `ClientIP` |

These settings are rejected: `privileged` in init or sidecar containers, enabling `allowPrivilegeEscalation`, adding Linux capabilities, choosing a custom ServiceAccount or auto-mounting its token, Service types other than `ClusterIP`, and setting `externalTrafficPolicy`.

## Source builds

| Field | Format | Default |
| --- | --- | --- |
| Dockerfile path | Path relative to the repository root | `Dockerfile` |
| Build context | Directory relative to the repository root | `.` |
| Target image | `repository:tag`; for variables see [Image Repository and Tag Templates](./image-templates) | Generated from the push credential template |
| Build Args | One `KEY=value` per line; see [Build Arguments and Build Variables](./build-variables) | Empty |
