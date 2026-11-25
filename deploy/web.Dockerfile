# Builds the admin and store apps and serves both from one unprivileged nginx.
#   docker build -f deploy/web.Dockerfile .

FROM node:22-alpine AS build
WORKDIR /src
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/apps/admin/package.json apps/admin/
COPY web/apps/store/package.json apps/store/
COPY web/packages/api/package.json packages/api/
COPY web/packages/ui/package.json packages/ui/
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm -r build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY deploy/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/admin/dist /usr/share/nginx/admin
COPY --from=build /src/apps/store/dist /usr/share/nginx/store
EXPOSE 8080
