# syntax=docker.io/docker/dockerfile:1

# ---- vendor: build the offline mermaid + ELK bundle -------------------------
FROM docker.io/library/node:26-alpine AS vendor
WORKDIR /vendor
COPY mermaid/package.json mermaid/package-lock.json mermaid/entry.js ./
RUN npm ci --no-audit --no-fund --legacy-peer-deps \
 && npx esbuild entry.js --bundle --format=esm --minify --outfile=mermaid-bundle.min.mjs

# ---- build: compile the static binary --------------------------------------
FROM docker.io/library/golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/okf-wiki ./cmd/okf-wiki

# ---- runtime ----------------------------------------------------------------
FROM docker.io/library/alpine:3.24
RUN adduser -D -u 10001 okf
COPY --from=build /out/okf-wiki /usr/local/bin/okf-wiki
COPY --from=vendor /vendor/mermaid-bundle.min.mjs /app/vendor/mermaid-bundle.min.mjs
USER okf
ENV OKF_WIKI_ADDR=0.0.0.0:8080 \
    OKF_WIKI_OUT=/tmp/okf-wiki \
    OKF_WIKI_VENDOR=/app/vendor

# A theme is a read-only bind mount plus OKF_WIKI_THEME=/theme. It is not baked
# in: the image ships the embedded default and an override layers over it at
# render time, so re-skinning never needs a rebuild.
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
  CMD wget -q -O- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1
ENTRYPOINT ["okf-wiki"]
CMD ["serve"]
