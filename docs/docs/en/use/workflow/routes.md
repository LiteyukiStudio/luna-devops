# Configure an Access Route

An access route sends a domain and path to a released application Service. Services used only inside the cluster do not need one.

## Create an access route

1. On the application's **Access routes** tab, select **Create access route** and choose the deploy config to expose.
2. Enter the domain (or only a prefix with a domain suffix), path, and target port.
3. Choose HTTP or the TLS mode that matches the real network path, then submit.

The entry Gateway defaults to the shared Gateway configured for the runtime cluster. Expand **Advanced settings** in the form to override the Gateway, path matching, request/response headers, URL rewrite, or redirect.

## Choose TLS

| Scenario | Choice |
| --- | --- |
| CDN or upstream proxy already terminates HTTPS | Upstream proxy terminates TLS |
| Gateway uses an existing certificate | Gateway terminates TLS |
| cert-manager HTTP-01 is configured | HTTP Challenge certificate |
| DNS-01 wildcard certificate is configured | Existing wildcard certificate |

Luna DevOps uses existing Gateway, Issuer, and certificate resources. It does not create ACME accounts or DNS Provider credentials.

After creation, verify provisioning, DNS, certificate, Service port, and application health, then make a real request with a browser or `curl`. If an application generates an incorrect `http` callback, ask the administrator to verify the external-protocol headers across the CDN, reverse proxy, and Gateway.

After you delete an access route, application, or project space, Luna DevOps removes the HTTP Challenge certificates and certificate Secrets issued for those routes and drops the matching certificate references from the Gateway. A certificate Secret stays while another access route for the same domain still uses it. The platform-issued wildcard certificate is removed once no platform access route remains in the cluster. Disabling an access route keeps its certificate.
