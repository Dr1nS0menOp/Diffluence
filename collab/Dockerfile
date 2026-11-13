# SPDX-License-Identifier: Apache-2.0
FROM node:22-slim@sha256:e9f565d32859f0cbfb52125eafe8be39390f24dac302dc3cdf566e8ba3ab21ee AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev

FROM node:22-slim@sha256:e9f565d32859f0cbfb52125eafe8be39390f24dac302dc3cdf566e8ba3ab21ee
# The service runs on node alone. npm and corepack, with the packages they bring, stay in
# the stage above, so the image ships only what runs (spec 067).
RUN rm -rf /usr/local/lib/node_modules /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY package.json server.mjs access.mjs document.mjs presence.mjs ./
USER node
EXPOSE 1234
CMD ["node", "server.mjs"]
