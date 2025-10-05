# Conduit

A lightweight tunneling service that enables secure connection forwarding through a remote server.

**How:** Deploy Conduit on a public server (e.g., `https://conduit.example.com`) to create tunnels from local services to the internet. Simply point a tunnel to your local endpoint (such as `localhost:8000`) and assign it a custom subdomain identifier like `black-fox-123`. Your local service becomes instantly accessible at `https://black-fox-123.conduit.example.com`.

**Key Benefits:**

- Expose local development servers to the internet
- Share work-in-progress applications with clients or teammates
- Test webhooks and external integrations
- Bypass firewall restrictions for remote access

Perfect for developers who need quick, temporary public access to local services without complex networking setup.

### Example

![Example](https://gitea.va.reichard.io/evan/conduit/raw/branch/main/assets/example.gif)
