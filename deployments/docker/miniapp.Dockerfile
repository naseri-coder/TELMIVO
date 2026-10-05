FROM node:24-bookworm-slim AS build
WORKDIR /src
COPY web/miniapp/package*.json ./
RUN npm ci
COPY web/miniapp ./
RUN npm run build
FROM nginxinc/nginx-unprivileged:stable-alpine
COPY deployments/reverse-proxy/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
