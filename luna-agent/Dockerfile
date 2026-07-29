FROM node:24-alpine AS build
ENV CI=true
WORKDIR /workspace/luna-agent
RUN corepack enable
COPY luna-agent/package.json luna-agent/pnpm-lock.yaml luna-agent/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY luna-agent/ ./
RUN pnpm build && pnpm prune --prod

FROM node:24-alpine AS runtime
ENV NODE_ENV=production
WORKDIR /app
RUN addgroup -S -g 10001 luna && adduser -S -D -H -u 10001 -G luna luna \
  && mkdir -p /tmp/luna-agent && chown -R luna:luna /app /tmp/luna-agent
COPY --from=build --chown=luna:luna /workspace/luna-agent/package.json ./package.json
COPY --from=build --chown=luna:luna /workspace/luna-agent/node_modules ./node_modules
COPY --from=build --chown=luna:luna /workspace/luna-agent/dist ./dist
COPY --from=build --chown=luna:luna /workspace/luna-agent/skills ./skills
USER 10001:10001
EXPOSE 8091
CMD ["node", "dist/index.js"]
