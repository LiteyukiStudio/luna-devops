# Add Base Resources

Administrators should prepare these resources so users can build or deploy immediately after creating an application:

```text
Runtime cluster → Registry → Git Provider OAuth
```

## 1. Add a runtime cluster

Add a Kubernetes cluster under **Runtime Clusters**, paste a kubeconfig reachable by both API and Worker, and test the connection.

- The kubeconfig API server must use HTTPS.
- When Luna DevOps runs in containers, do not use a loopback address reachable only from the host.
- Configure the default Gateway, domain suffix, and TLS when required. Luna DevOps does not create ACME accounts or DNS Provider credentials.

Under **Resource allocation policy**, set the percentages of the application quota used for CPU request, memory request, CPU limit, and memory limit. The defaults are `10% / 25% / 100% / 100%`; each value accepts 0–100, and `0%` omits that Kubernetes field. When a limit is enabled, its request percentage cannot be greater. Request-only and limit-only policies are both valid. Policy changes affect later redeployments and do not rewrite running workloads.

The Kubernetes scheduler still reserves and schedules capacity from requests. Limits are runtime ceilings: exceeding a CPU limit normally throttles the container, while exceeding a memory limit may cause an OOMKill.

The cluster list reads current Kubernetes state about every 10 seconds. The CPU and memory rings show effective requests from scheduled, non-terminal Pods divided by total Node allocatable capacity. Their tooltips also show actual Node usage when the Metrics API is available. The pressure level combines CPU and memory request allocation with actual usage, gives memory slightly more weight, and prevents one nearly saturated dimension from being hidden by an average: below 20 is **Idle**, 20–44.9 **Light**, 45–69.9 **Moderate**, 70–89.9 **Heavy**, and 90 or above **Full**. This is a capacity overview; Kubernetes still makes the final scheduling decision from taints, affinity, topology, and per-Node headroom.

Before deleting a cluster, migrate or remove every deployment that references it.

## 2. Add a registry

Add a Harbor, Gitea Registry, DockerHub, or generic OCI Registry and test the connection.

- **Source builds** need a push or pull-and-push credential visible to the current user or project space.
- **Existing-image deployments** require image-pull access from the target cluster.
- Limit credentials to the necessary users or project spaces. Deleting a registry also deletes its credentials.

## 3. Configure Git Provider OAuth

Open **Code Repositories → Git Providers**, create a GitHub or Gitea Provider, choose OAuth, and create an OAuth App on the Git host using the callback URL shown by Luna DevOps.

1. Enter the Provider type and service URL.
2. Copy the callback URL into the OAuth App.
3. Save the Client ID and Client Secret in the Provider.
4. Test the connection and complete one OAuth authorization as a regular user.

GitLab currently works best with PAT credentials. Teams that deploy existing images only can configure a Git Provider later.

## Verify the setup

Use a non-administrator test account to confirm that it can select the cluster and registry, authorize a Git account through OAuth, and search repositories. Then continue to [Daily Delivery](/use/workflow).
