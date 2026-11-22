<p align="center">
  <img src="web/public/luna-devops-logo.svg" width="112" alt="Luna DevOps" />
</p>

<h1 align="center">Luna DevOps</h1>

<p align="center">
  面向个人开发者与小团队的开源应用交付平台。
</p>

<p align="center">
  <strong>简体中文</strong> · <a href="README_EN.md">English</a>
</p>

<p align="center">
  <a href="https://luna-devops.liteyuki.org/">官方文档</a>
  ·
  <a href="https://luna-devops.liteyuki.org/start/install/kubernetes">安装指南</a>
  ·
  <a href="https://github.com/LiteyukiStudio/luna-devops/releases">版本发布</a>
  ·
  <a href="LICENSE">MIT License</a>
</p>

<p align="center">
  <img src=".github/assets/luna-devops-banner-v4.png" alt="Luna DevOps 概览" />
</p>

自己维护集群跑服务时，从写完代码到真正上线，中间往往有一堆重复琐事：打包镜像、推送仓库、调整部署配置、绑定域名、申请证书、翻日志查报错。重型平台部署和维护成本太高，全靠脚本和零散配置文件又不好协作。

Luna DevOps 把这些日常高频操作收进一个轻便的控制台里。接上现有的代码仓库、镜像站和集群后，日常只需维护好代码和基础配置，构建、发布、回滚、配域名和看状态都在同一处完成。它不绑定特定云厂商，也不会替代你原本在用的 GitHub、Gitea 或 Harbor。

## 能做什么

- 连接 GitHub / Gitea 仓库自动完成镜像构建与上线，也可以直接部署现成镜像，或从应用市场模板创建常用服务。
- 每次构建和发布都保留历史记录，遇到问题可随时回退旧版本；在浏览器里直接查看运行状态、实时日志、数据卷和服务连接。
- 为对外应用绑定自定义域名并自动申请、续期 HTTPS 证书；不需要公开访问的服务可设为仅项目内互通。
- 按项目空间管理应用、成员权限、告警通知与资源账单，多人或多项目共用一套集群时边界清晰。
- 内置网页端 AI 助手，同时支持通过 MCP 接入 Claude Code、Cursor 等本地工具，在对话中查状态、排查报错或准备配置变更。

## 开始使用

完整的安装步骤、初始化配置和日常操作指南统一维护在 **[官方文档站](https://luna-devops.liteyuki.org/)**：

- **部署安装**：[Kubernetes (Helm)](https://luna-devops.liteyuki.org/start/install/kubernetes) · [Docker Compose](https://luna-devops.liteyuki.org/start/install/docker-compose)
- **初始化平台**：[首次登录](https://luna-devops.liteyuki.org/start/first-login) · [添加基础资源](https://luna-devops.liteyuki.org/start/connect-resources)
- **日常使用**：[应用交付流程](https://luna-devops.liteyuki.org/use/workflow) · [应用市场](https://luna-devops.liteyuki.org/use/app-marketplace) · [AI 助手](https://luna-devops.liteyuki.org/use/ai-assistant) · [MCP 接入](https://luna-devops.liteyuki.org/use/mcp)

## 反馈与参与

- Bug 反馈与功能建议欢迎提交至 [GitHub Issues](https://github.com/LiteyukiStudio/luna-devops/issues)。
- 本地开发环境搭建、仓库结构与代码贡献规范请查阅 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。第三方依赖以及 Luna DevOps / Liteyuki Studio 名称与标志的使用说明见[许可说明](https://luna-devops.liteyuki.org/reference/license)。
