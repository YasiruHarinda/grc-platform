# NOT IN USE on Choreo's React buildpack (see issue #90) — that buildpack
# builds and serves dist/ itself, so this Dockerfile never runs there. Kept,
# with index.js, so the component can be switched back to the Dockerfile
# buildpack without a code change.
#
# Stage 1: build the React app
FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci --legacy-peer-deps

COPY . .
RUN npm run build

# Stage 2: serve dist/ via the Node proxy server (index.js)
FROM node:20-alpine

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder --chown=app:app /app/dist ./dist
COPY --chown=app:app package.json ./
COPY --chown=app:app index.js ./

EXPOSE 8080

USER app

CMD ["node", "index.js"]
