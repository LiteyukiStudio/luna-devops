# Add Base Resources

Administrators should prepare these resources so users can build or deploy immediately after creating an application:

```text
Runtime cluster → Registry → Git Provider OAuth
```

## Choose global-resource visibility

Cross-project lists for runtime clusters, registries, Git providers and accounts, images and credentials, and build variable sets default to **Related to me**. This keeps global resources, resources owned by the current account, and resources bound to project spaces where the account is a member. Platform administrators switch to **All** only for an explicit platform-wide inventory. When the target project space is known, select it as well; the project-space filter narrows results within the current visibility.

## 1. Add a runtime cluster

Add a Kubernetes cluster under **Runtime Clusters**, paste a kubeconfig reachable by both API and Worker, and test the connection.

- The kubeconfig API server must use HTTPS.
- When Luna DevOps runs in containers, do not use a loopback address reachable only from the host.
- Configure the default Gateway, domain suffix, and TLS when required. Luna DevOps does not create ACME accounts or DNS Provider credentials.

Under **Resource allocation policy**, set the percentages of the application quota used for CPU request, memory request, CPU limit, and memory limit. The defaults are `10% / 25% / 100% / 100%`; each value accepts 0–100, and `0%` omits that Kubernetes field. When a limit is enabled, its request percentage cannot be greater. Request-only and limit-only policies are both valid. Policy changes affect later redeployments and do not rewrite running workloads.

The Kubernetes scheduler still reserves and schedules capacity from requests. Limits are runtime ceilings: exceeding a CPU limit normally throttles the container, while exceeding a memory limit may cause an OOMKill.

The cluster list reads current Kubernetes state about every 10 seconds. The CPU and memory rings show effective requests from scheduled, non-terminal Pods divided by total Node allocatable capacity. Their tooltips also show actual Node usage when the Metrics API is available. The pressure level combines CPU and memory request allocation with actual usage, gives memory slightly more weight, and prevents one nearly saturated dimension from being hidden by an average: below 20 is **Idle**, 20–44.9 **Light**, 45–69.9 **Moderate**, 70–89.9 **Heavy**, and 90 or above **Full**. This is a capacity overview; Kubernetes still makes the final scheduling decision from taints, affinity, topology, and per-Node headroom.

### Network isolation

The platform applies managed NetworkPolicies in every project space namespace to stop tenant applications from reaching each other or the network the cluster runs in.

**Prerequisites**: the cluster network plugin must enforce NetworkPolicy, for example Calico, Cilium, or the k3s default network. With a plugin that does not enforce policies, such as Flannel alone, the policy objects are created but have no effect. The cluster credential must be able to read Nodes, Namespaces, and the `default/kubernetes` Endpoints, and manage NetworkPolicies in project namespaces.

**Behavior**:

- Ingress: only Pods in the same project space, the gateway controller namespace, and cluster nodes (kubelet probes and host-network components) can reach applications. Other project spaces cannot connect directly.
- Egress: the same project space, the gateway, and the public internet are allowed, and DNS is unrestricted. Private and reserved ranges (such as `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10`, `169.254.0.0/16`, and their IPv6 equivalents), cluster node addresses, and the Kubernetes API are denied.
- To call an application in another project space, use its route domain through the gateway.
- Build egress restrictions apply independently and are not widened by these rules.

**Configuration** (edit cluster → Network isolation):

| Field | Purpose | Format and range |
| --- | --- | --- |
| Additional blocked egress CIDRs | Added on top of the default denied ranges, such as a data center public range or the public address of a router or other private service | One CIDR per line; `/0` is rejected; at most 256 entries |
| Gateway controller namespace | Namespace of the gateway controller Pods. Only traffic from here can enter project spaces | A Kubernetes namespace name; empty uses the gateway namespace |

> If the gateway controller namespace is wrong, no route can reach any application. Confirm the namespace where the gateway controller Pods actually run before saving.

After saving, the platform reapplies the policies immediately and compares them every 5 minutes; managed policies edited or deleted in the cluster are restored. When editing a cluster, **Isolation status** shows the live result: **Synced** means every project space matches the platform configuration, **Drifted** means some project spaces are waiting for automatic repair, and **Unavailable** means the cluster could not be read; check the cluster credential and connectivity as indicated.

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
