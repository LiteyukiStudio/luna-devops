<p align="center">
  <img src="web/public/luna-devops-logo.svg" width="112" alt="Luna DevOps" />
</p>

<h1 align="center">Luna DevOps</h1>

<p align="center">
  Open-source application delivery platform for solo developers and small teams.
</p>

<p align="center">
  <a href="README.md">简体中文</a> · <strong>English</strong>
</p>

<p align="center">
  <a href="https://luna-devops.liteyuki.org/en/">Documentation</a>
  ·
  <a href="https://luna-devops.liteyuki.org/en/start/install/kubernetes">Install</a>
  ·
  <a href="https://github.com/LiteyukiStudio/luna-devops/releases">Releases</a>
  ·
  <a href="LICENSE">MIT License</a>
</p>

<p align="center">
  <img src=".github/assets/luna-devops-banner-v4.png" alt="Luna DevOps overview" />
</p>

Running apps on your own cluster usually comes with a long list of chores between pushing code and going live: building images, pushing to a registry, maintaining deployment configs, wiring up domains, renewing certificates, and digging through logs when something breaks. Enterprise platforms are often too heavy to run for a small team, while hand-rolled scripts scatter quickly across repositories.

Luna DevOps brings those everyday steps into a lightweight web console. Connect your existing Git host, image registry, and cluster, then handle builds, rollouts, rollbacks, custom domains, and live status checks in one place. There is no cloud lock-in, and it works alongside the GitHub, Gitea, or Harbor setup you already use.

## What You Can Do

- Deploy straight from GitHub or Gitea repositories, run an existing container image, or launch common services from marketplace templates.
- Keep a clear history of builds and releases, roll back to a previous version when needed, and check live status, logs, persistent volumes, and service connections in the browser.
- Attach custom domains with automated HTTPS certificates for public apps, or keep internal services private within a project space.
- Organize apps by project space with member roles, notifications, resource quotas, and usage billing so multiple projects can share a cluster cleanly.
- Use the built-in web AI assistant, or connect local tools like Claude Code and Cursor over MCP to check app health, troubleshoot failures, and prepare changes in chat.

## Getting Started

Setup guides, configuration references, and usage walkthroughs live on the **[Documentation Site](https://luna-devops.liteyuki.org/en/)**:

- **Install**: [Kubernetes (Helm)](https://luna-devops.liteyuki.org/en/start/install/kubernetes) · [Docker Compose](https://luna-devops.liteyuki.org/en/start/install/docker-compose)
- **Initial Setup**: [First Login](https://luna-devops.liteyuki.org/en/start/first-login) · [Connect Resources](https://luna-devops.liteyuki.org/en/start/connect-resources)
- **Day-to-Day Use**: [Delivery Workflow](https://luna-devops.liteyuki.org/en/use/workflow) · [App Marketplace](https://luna-devops.liteyuki.org/en/use/app-marketplace) · [AI Assistant](https://luna-devops.liteyuki.org/en/use/ai-assistant) · [MCP Integration](https://luna-devops.liteyuki.org/en/use/mcp)

## Community & Contributing

- Found a bug or have an idea? Open an issue on [GitHub Issues](https://github.com/LiteyukiStudio/luna-devops/issues).
- Looking to run from source or contribute code? See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Released under the [MIT License](LICENSE). Third-party dependencies and Luna DevOps / Liteyuki Studio brand assets remain subject to their respective terms; see the [license notes](https://luna-devops.liteyuki.org/en/reference/license).
