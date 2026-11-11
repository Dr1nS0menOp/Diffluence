# SPDX-License-Identifier: Apache-2.0
# Base images are pinned by digest, with the tag kept for reading; Renovate moves the
# digests (OPS-096).
FROM node:22-bookworm-slim@sha256:18052396bf1d0c6e0a8c58181b5c1457da19409efbd5032a3f4d86974e74d13c AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-fund --no-audit
COPY web/ ./
RUN npx vite build

FROM golang:1.26@sha256:5ebd6e7dc13f9e273ed9470679eb295d8b6e7de3b6379c8a5e17a9e6c3eabd38 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/webui/dist/ internal/webui/dist/
ARG VERSION=dev
# The date of the commit (git log -1 --format=%cs); empty for a developer build (spec 051).
ARG BUILD_DATE=
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/diffluence/diffluence/internal/version.Version=${VERSION} -X github.com/diffluence/diffluence/internal/version.Date=${BUILD_DATE}" \
      -o /out/diffluence ./cmd/diffluence
RUN mkdir -p /out/data

# The backup service of compose.yaml (spec 054): pg_dump of the same major version as the
# database, gpg to encrypt, and the diffluence binary of this build to note each backup.
# docker build . skips this stage; compose.yaml builds it with target: backup.
FROM postgres:16 AS backup
RUN apt-get update \
 && apt-get install -y --no-install-recommends gnupg \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/diffluence /usr/local/bin/diffluence
COPY deploy/backup/backup-service.sh /usr/local/bin/diffluence-backup
# Through bash, so the file needs no executable bit from the checkout.
ENTRYPOINT ["bash", "/usr/local/bin/diffluence-backup"]
CMD ["loop"]

# Distroless: no shell, no package manager, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
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
