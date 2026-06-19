---
description: Liteyuki DevOps documentation home for shipping applications from source code to reachable services.
pageType: home

hero:
  name: Liteyuki DevOps
  text: Code once, deploy anywhere.
  tagline: From local startup to production release, keep builds, deployments, and access in one clear delivery path.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: See Features
      link: /operations/deployment
  image:
    src: /brand/mascot-liteyuki-catgirl-alpha.webp
    alt: Liteyuki DevOps mascot
features:
  - title: "Start: Docker Compose deploy"
    details: Pull liteyukistudio images by default, run docker compose up -d, then open localhost:8088 to enter the console.
    link: /guide/getting-started
  - title: "Use: feature map"
    details: Project spaces, applications, deployment targets, Releases, and routes follow the daily delivery workflow.
    link: /operations/deployment
  - title: "Use: settings and status"
    details: Git providers, registries, runtime clusters, and secrets are managed separately, with status pages for build, release, and access issues.
    link: /operations/configuration
  - title: "Develop: local workflow"
    details: Developers can use docker-compose-dev.yaml for dependencies, run API and Web on the host, and update docs with feature changes.
    link: /developer/architecture
---
