# SPDX-License-Identifier: Apache-2.0
FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npx vite build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/webui/dist/ internal/webui/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/diffluence/diffluence/internal/version.Version=${VERSION}" \
      -o /out/diffluence ./cmd/diffluence
RUN mkdir -p /out/data

# Distroless: no shell, no package manager, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/diffluence /usr/local/bin/diffluence
COPY --from=build --chown=nonroot:nonroot /out/data /var/lib/diffluence
ENV DIFFLUENCE_HTTP_ADDR=:8080 \
    DIFFLUENCE_DATA_DIR=/var/lib/diffluence
USER nonroot:nonroot
EXPOSE 8080
VOLUME ["/var/lib/diffluence"]
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s CMD ["/usr/local/bin/diffluence", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/diffluence"]
CMD ["serve"]
