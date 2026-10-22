FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm test && npm run build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build --chown=node:node /app/package.json ./
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --from=build --chown=node:node /app/.svelte-kit ./.svelte-kit
COPY --from=build --chown=node:node /app/build ./build
COPY --from=build --chown=node:node /app/src ./src
COPY --from=build --chown=node:node /app/e2e ./e2e
COPY --from=build --chown=node:node /app/vite.config.ts ./vite.config.ts
COPY --from=build --chown=node:node /app/svelte.config.js ./svelte.config.js
COPY --from=build --chown=node:node /app/tsconfig.json ./tsconfig.json
RUN rm -rf /app/node_modules/.vite-temp && ln -s /tmp /app/node_modules/.vite-temp
USER node
EXPOSE 3000
CMD ["node", "build"]
