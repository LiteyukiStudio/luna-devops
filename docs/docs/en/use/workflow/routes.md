# Configure a Route

A route sends a domain and path to a released application Service. Services used only inside the cluster do not need one.

## Create a route

1. Select the project space, application, and stage.
2. Select a Gateway available in the cluster.
3. Enter the domain, path, and target port.
4. Choose HTTP or the TLS mode that matches the real network path.

## Choose TLS

| Scenario | Choice |
| --- | --- |
| CDN or upstream proxy already terminates HTTPS | Upstream proxy terminates TLS |
| Gateway uses an existing certificate | Gateway terminates TLS |
| cert-manager HTTP-01 is configured | HTTP Challenge certificate |
| DNS-01 wildcard certificate is configured | Existing wildcard certificate |

Luna DevOps uses existing Gateway, Issuer, and certificate resources. It does not create ACME accounts or DNS Provider credentials.

After creation, verify provisioning, DNS, certificate, Service port, and application health, then make a real request with a browser or `curl`. If an application generates an incorrect `http` callback, ask the administrator to verify the external-protocol headers across the CDN, reverse proxy, and Gateway.
