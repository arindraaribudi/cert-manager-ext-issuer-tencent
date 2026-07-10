# syntax=docker/dockerfile:1.6

ARG VCS_REF=unknown
ARG VCS_VERSION=dev
ARG BUILD_DATE=1970-01-01T00:00:00Z
ARG REPO_URL=https://github.com/arindraaribudi/cert-manager-ext-issuer-tencent
ARG SBOM_URL="https://github.com/arindraaribudi/cert-manager-ext-issuer-tencent/releases/download/${VCS_VERSION}/sbom.spdx.json"

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/manager ./cmd/main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/manager /manager
USER 65532:65532
ENTRYPOINT ["/manager"]

LABEL org.opencontainers.image.title="cert-manager-ext-issuer-tencent"
LABEL org.opencontainers.image.description="External issuer for cert-manager using Tencent Cloud SSL Certificates API."
LABEL org.opencontainers.image.source="${REPO_URL}"
LABEL org.opencontainers.image.url="${REPO_URL}"
LABEL org.opencontainers.image.documentation="${REPO_URL}#readme"
LABEL org.opencontainers.image.version="${VCS_VERSION}"
LABEL org.opencontainers.image.revision="${VCS_REF}"
LABEL org.opencontainers.image.created="${BUILD_DATE}"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.vendor="ArindraAribudi"
LABEL org.opencontainers.image.sbom="${SBOM_URL}"