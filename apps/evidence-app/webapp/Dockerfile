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
