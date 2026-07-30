---
description: Luna DevOps 文档首页，帮助个人开发者和小团队完成从代码到上线的应用交付。
pageType: home

hero:
  name: Luna DevOps
  text: Code once, deploy anywhere.
  tagline: 轻松几步部署自己的项目，为小型团队和企业提供一套清晰的 DevOps 方案。
  actions:
    - theme: brand
      text: 快速开始
      link: /start/install/kubernetes
    - theme: alt
      text: 部署项目
      link: /start/first-project
  image:
    src: /brand/mascot-luna-catgirl-alpha.webp
    alt: Luna DevOps mascot
features:
  - title: Kubernetes (Helm) 部署
    details: 已经有 Kubernetes 或 K3s？用 Helm 一次安装 API、Worker、PostgreSQL 和 Redis。
    link: /start/install/kubernetes
  - title: Docker Compose 部署
    details: 想先在一台机器上试用？一条 docker compose up -d 就能启动完整平台。
    link: /start/install/docker-compose
  - title: 部署一个 Web 项目
    details: 跟着完整示例创建项目空间、构建镜像、发布应用，再为它配置可访问的域名。
    link: /start/first-project
  - title: 连接外部系统
    details: 先连接运行集群和镜像站，确认基本部署可用，再接入 Git 和自动化。
    link: /start/connect-resources
  - title: 排查失败状态
    details: 不知道问题在哪？按构建、发布、集群和访问入口的顺序逐段检查。
    link: /use/diagnostics
---
