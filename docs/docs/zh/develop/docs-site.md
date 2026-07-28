# 维护文档站

文档站使用 Rspress 2，源码位于 `docs/`。

## 常用命令

```bash
pnpm --dir docs install
pnpm --dir docs dev
pnpm --dir docs build
pnpm --dir docs preview
```

## 目录结构

```text
docs/
  rspress.config.ts
  theme/
  docs/
    public/
    zh/
    en/
```

`zh/` 和 `en/` 使用相同的目录结构。新增、删除或移动页面时，两种语言要一起调整。

## 资源

- Logo：`docs/docs/public/luna-devops-logo.svg`
- 吉祥物：`docs/docs/public/brand/mascot-luna-catgirl-alpha.webp`

资源来自主项目前端的品牌素材，文档站构建时以静态资源发布。

## 内容风格

- 面向用户时先讲“现在要做什么”，再补充“为什么这样设计”。
- 命令要能直接复制执行。
- 开始页优先帮助用户完成部署；使用页解释功能；开发页再讲代码和贡献方式。
- 危险操作必须写清影响范围。
- 句子尽量短，少用内部汇报口吻；可以亲切，但不能牺牲准确性。
