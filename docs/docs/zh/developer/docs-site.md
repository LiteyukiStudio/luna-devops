# 文档站维护

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

`zh/` 和 `en/` 保持同构，方便中英内容同步维护。

## 资源

- Logo：`docs/docs/public/liteyuki-logo.svg`
- 吉祥物：`docs/docs/public/brand/mascot-liteyuki-catgirl-alpha.webp`

资源来自主项目前端的品牌素材，文档站构建时以静态资源发布。

## 内容风格

- 面向用户时先讲“要做什么”，再讲“为什么这样设计”。
- 命令要能直接复制执行。
- 开始页优先帮助用户完成部署；使用页解释功能；开发页再讲代码和贡献方式。
- 危险操作必须写清影响范围。
- 可以亲切活泼，但不要牺牲准确性。
