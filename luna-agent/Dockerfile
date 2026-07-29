FROM node:24-alpine AS build
WORKDIR /workspace
RUN corepack enable
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.base.json ./
COPY luna-agent/package.json luna-agent/package.json
RUN pnpm install --frozen-lockfile --filter @luna-devops/agent...
COPY luna-agent luna-agent
RUN pnpm --dir luna-agent build && pnpm deploy --legacy --filter @luna-devops/agent --prod /out

FROM node:24-alpine AS runtime
ENV NODE_ENV=production
WORKDIR /app
RUN addgroup -S -g 10001 luna && adduser -S -D -H -u 10001 -G luna luna \
  && mkdir -p /tmp/luna-agent && chown -R luna:luna /app /tmp/luna-agent
COPY --from=build --chown=luna:luna /out ./
COPY --from=build --chown=luna:luna /workspace/luna-agent/dist ./dist
USER 10001:10001
EXPOSE 8091
CMD ["node", "dist/index.js"]
