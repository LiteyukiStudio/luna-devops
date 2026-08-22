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
