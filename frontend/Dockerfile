# syntax=docker/dockerfile:1

# ── Build ────────────────────────────────────────────────────────────────────
FROM node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
# Public build-time value: it is embedded in the browser bundle, never a secret.
ARG VITE_API_URL=""
ENV VITE_API_URL=${VITE_API_URL}
# a+rX: files copied from public/ keep the build context mode (may be 0600).
RUN npm run build && chmod -R a+rX dist

# ── Runtime: nginx without root on :8080 ─────────────────────────────────────
FROM nginxinc/nginx-unprivileged:1.29-alpine@sha256:0c79d56aee561a1d81c63f00eee5fb5fe29279560cdc55e91425133104c7fbe6
COPY --from=build /app/dist /usr/share/nginx/html
COPY --chmod=0644 nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
  CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/"]
