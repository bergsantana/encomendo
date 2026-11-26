# Vite dev servers for the admin and store apps, for docker-compose.dev.yml.
# Dependencies are installed in the image; `docker compose watch` syncs source
# changes into /app, and Vite's hot reload does the rest.

FROM node:26-alpine
WORKDIR /app
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/apps/admin/package.json apps/admin/
COPY web/apps/store/package.json apps/store/
COPY web/packages/api/package.json packages/api/
COPY web/packages/ui/package.json packages/ui/
RUN pnpm install --frozen-lockfile
COPY web/ ./
