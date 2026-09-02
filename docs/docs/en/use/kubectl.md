# Manage Project Resources with kubectl

Luna DevOps can issue project-space or application-scoped kubeconfig files. `kubectl` connects to the Luna DevOps Kubernetes API-compatible gateway, not to the runtime cluster's kube-apiserver. The real cluster address, management kubeconfig, and upstream ServiceAccount token are never sent to your machine.

## Prerequisites

- Your account is still an active member of the target project space.
- The runtime cluster is Active, a platform administrator has enabled its kubectl gateway, and the live gateway status is Ready.
- The administrator has configured HTTPS for the public platform URL and a reverse proxy that supports long-lived connections. See [Kubernetes (Helm) Deployment](/en/start/install/kubernetes#configure-the-kubectl-gateway-reverse-proxy).
- Install a `kubectl` version validated for the current platform release. See [Compatibility](/en/reference/compatibility) and the current Release notes for the exact range.

## Get kubeconfig from the console

1. Open the target project space and select **kubectl access**.
2. Select **Create kubeconfig**, choose an available runtime cluster, and optionally restrict access to one application.
3. Choose the minimum scopes and a validity of 1, 7, or 30 days, then create the credential.
4. Save the automatically downloaded kubeconfig immediately. The plaintext is returned once and cannot be viewed or downloaded again after the dialog closes.
5. Confirm that only your account can read the file:

```bash
chmod 600 "$HOME/.kube/luna-project.yaml"
```

The console creates one context at a time. To review or revoke one later, open **Account settings → kubectl credentials**. The page lists credentials created by your account, supports name and status filters, and links back to project spaces when no credentials exist. You can review context metadata there, but cannot recover the token or kubeconfig. Revocation also invalidates files that were already downloaded.

## Write or merge with Luna CLI

The dedicated Luna CLI commands never print kubeconfig or tokens to normal stdout. Write a new file:

```bash
luna kubeconfig write \
  credentialName=dev-read \
  context=prj_example:clu_example \
  scope=read \
  expiresInDays=7 \
  destination="$HOME/.kube/luna-dev-read.yaml"
```

Merge into an existing kubeconfig:

```bash
luna kubeconfig merge \
  credentialName=dev-tools \
  context=prj_example:clu_example:app_example \
  scope=read \
  scope=connect \
  expiresInDays=1 \
  destination="$HOME/.kube/config"
```

Repeat `context=...` to issue multiple contexts at once. Its format is `projectId:runtimeClusterId[:applicationId]`; `scope=read|write|connect` maps to `kube:read|kube:write|kube:connect`. CLI validates YAML, destination permissions, and predictable name conflicts before an atomic `0600` write. Add `replaceConflicts=true` only after confirming that you intend to replace same-name entries with different content.

Creating and managing these credentials requires `token:manage` on the current Luna CLI OAuth session. Follow the reauthorization prompt if it is missing; do not substitute an ordinary access token for a Kube Credential.

## Select a context and verify access

Use a downloaded file separately first so you do not accidentally alter your default kubeconfig:

```bash
export KUBECONFIG="$HOME/.kube/luna-dev-read.yaml"
kubectl config get-contexts
kubectl config current-context
kubectl auth whoami
kubectl auth can-i get deployments
kubectl get deployments
```

Generated context names use stable resource IDs and fix the target project's Kubernetes Namespace. Changing `-n` cannot cross that boundary, and `-A` is rejected. An application-scoped context also enforces that application's ownership label.

## How scopes are evaluated

| Scope | Main use | Behavior |
| --- | --- | --- |
| `kube:read` | Discovery, OpenAPI, `get/list/watch`, `describe`, `wait`, `top`, logs, and authorization checks | The current project role must still allow the corresponding read action; Secret values require a higher permission. |
| `kube:write` | `create/apply/edit/replace/patch/delete`, Scale, Rollout, and other writes | Includes `kube:read`, but never bypasses workload policy or Luna DevOps business actions. |
| `kube:connect` | `exec`, `attach`, `port-forward`, `cp`, and controlled Debug | Includes `kube:read`; connections still require Developer or higher, `deployment:exec`, and the project Web Console setting. |

Every request re-evaluates the credential, binding, project membership, current role, cluster state, Namespace, resource type, and object ownership. Issuing kubeconfig does not freeze permissions. A role downgrade, member removal, or application or cluster deactivation removes access from existing files. Established watches, logs, and connections are periodically revalidated and close within about 30 seconds after revocation.

## Supported behavior and fixed limits

Within the authorized boundary, the gateway supports standard Discovery and OpenAPI, reads and output formats, CRUD/Apply/Patch, watches, logs, Exec, Attach, Port-forward, `cp`, authorization checks, and Kubernetes-native Status, Table, pagination, and streaming behavior. Pure client features such as `kubectl config`, `completion`, `kustomize`, and plugins continue to run locally.

No additional scope can bypass these boundaries:

- Node, PV, cluster RBAC, CRD, webhook, CSR, APIService, and cluster-scoped Gateway administration are unavailable. `kubectl --as` is not supported.
- Namespace, ServiceAccount, and ServiceAccount token writes, Node Debug, resource `proxy`, and cross-Namespace references are denied.
- Workloads cannot use privileged mode, host namespaces, hostPath, hostPort, privilege escalation, added Linux capabilities, arbitrary ServiceAccounts, projected ServiceAccount tokens, or CSI secret providers.
- Services are limited to ClusterIP. Manage external traffic through Luna DevOps access entries.
- Secret values require an Owner or Admin role that currently has `secret:view_value`. A user with Exec access may still read secrets already injected into a container, so do not treat an Exec credential as ordinary read-only access.

For platform-created resources, the deployment configuration in Luna DevOps remains the desired state. Temporary kubectl changes can be overwritten by a later release, rollback, rebuild, or reconciliation. Kubernetes stores the desired state of resources created through kubectl; Luna DevOps includes them in project cleanup, observation, and runtime-resource billing.

## Troubleshooting

| Symptom | What to do |
| --- | --- |
| Creation says the gateway is Disabled, Reconciling, or Unavailable | Ask an administrator to enable the gateway under **Runtime clusters** and wait for Ready. If it remains unavailable, inspect runtime-cluster connectivity and the reconciliation task. |
| `401 Unauthorized` | The credential expired or was revoked, the user was disabled, or the file contains an ordinary access token. Create a new Kube Credential. |
| `403 Forbidden` | The current scope, project role, business action, resource catalog, or application boundary denies the operation. Check with `kubectl auth can-i`; do not try another Namespace as a bypass. |
| `404 NotFound` | The object does not exist or is outside this project/application binding. The gateway intentionally hides the existence of out-of-scope objects. |
| `422 Unprocessable Entity` | The Namespace, protected ownership labels, references, or workload security policy are invalid. Correct the manifest using the returned Kubernetes Status. |
| `429 TooManyRequests` | Request or stream concurrency limits were reached. Close idle watches, log followers, Exec sessions, or port forwards before retrying. |
| `503/504` | The runtime cluster, Secret Store, TokenRequest, or upstream API is temporarily unavailable. Retry later and ask an administrator to inspect cluster health. Application-scoped `kubectl top` also fails closed when the Metrics Provider cannot prove label isolation. |
| A watch, log stream, or connection periodically closes | Watches last up to 30 minutes; logs and connections last up to 2 hours. These are reauthorization boundaries, and clients can reconnect normally. |

When access is no longer needed, revoke it under **Account settings → kubectl credentials**, then securely delete the local file. Never commit kubeconfig to a repository or attach it to chat messages or tickets.
