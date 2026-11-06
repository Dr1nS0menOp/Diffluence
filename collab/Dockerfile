# SPDX-License-Identifier: Apache-2.0
FROM node:22-slim
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY server.mjs document.mjs ./
USER node
EXPOSE 1234
CMD ["node", "server.mjs"]
