# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS deps
WORKDIR /app

COPY package.json ./
COPY package-lock.json* pnpm-lock.yaml* yarn.lock* ./
RUN if [ -f pnpm-lock.yaml ]; then \
      corepack enable && pnpm install --frozen-lockfile; \
    elif [ -f package-lock.json ]; then \
      npm ci; \
    elif [ -f yarn.lock ]; then \
      corepack enable && yarn install --frozen-lockfile; \
    else \
      npm install; \
    fi

FROM deps AS build
WORKDIR /app

COPY . .
RUN npm run build

FROM node:22-alpine AS runtime
WORKDIR /app

ENV NODE_ENV=production

COPY package.json ./
COPY package-lock.json* pnpm-lock.yaml* yarn.lock* ./
RUN if [ -f pnpm-lock.yaml ]; then \
      corepack enable && pnpm install --prod --frozen-lockfile; \
    elif [ -f package-lock.json ]; then \
      npm ci --omit=dev; \
    elif [ -f yarn.lock ]; then \
      corepack enable && yarn install --production --frozen-lockfile; \
    else \
      npm install --omit=dev; \
    fi

COPY --from=build /app/build ./build

EXPOSE 3000

# Expected server entrypoint after TypeScript build.
CMD ["node", "build/index.js"]
