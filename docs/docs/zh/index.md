---
description: Liteyuki DevOps 文档首页，帮助个人开发者和小团队完成从代码到上线的应用交付。
pageType: home

hero:
  name: Liteyuki DevOps
  text: Code once, deploy anywhere.
  tagline: 从本地启动到线上发布，把构建、部署和访问管理收进一条清晰的交付路径。
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/getting-started
    - theme: alt
      text: 查看功能
      link: /operations/deployment
  image:
    src: /brand/mascot-liteyuki-catgirl-alpha.webp
    alt: Liteyuki DevOps mascot
features:
  - title: 开始：Docker Compose 部署
    details: 默认拉取 liteyukistudio 镜像，用 docker compose up -d 启动完整平台，打开 localhost:8088 即可进入控制台。
    link: /guide/getting-started
  - title: 使用：功能地图
    details: 用项目空间管理团队和资源，用应用、部署配置、Release 和访问入口串起日常交付。
    link: /operations/deployment
  - title: 使用：配置与排障
    details: Git Provider、镜像站、运行集群和密钥分开管理，状态页帮助你定位构建、发布和访问问题。
    link: /operations/configuration
  - title: 开发：本地工作流
    details: 开发者可以用 docker-compose-dev.yaml 承载依赖，在宿主机运行 API 和 Web，改动时同步维护文档站。
    link: /developer/architecture
---
