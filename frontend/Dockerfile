# syntax=docker/dockerfile:1.18

FROM node:24.21.0-bookworm-slim AS dependencies

RUN npm install --global pnpm@12.5.1
WORKDIR /source
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/package.json
RUN pnpm install --frozen-lockfile --filter @credar/frontend...

FROM dependencies AS build

ARG NEXT_PUBLIC_CREDAR_API_BASE_URL=http://localhost:8080
ENV NEXT_PUBLIC_CREDAR_API_BASE_URL=${NEXT_PUBLIC_CREDAR_API_BASE_URL}

COPY frontend/ ./frontend/
RUN pnpm --dir frontend build

FROM node:24.21.0-bookworm-slim AS runtime

ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000

WORKDIR /app
COPY --chown=node:node --from=build /source/frontend/.next/standalone ./
COPY --chown=node:node --from=build /source/frontend/.next/static ./frontend/.next/static

USER node
WORKDIR /app/frontend
EXPOSE 3000
CMD ["node", "server.js"]
