# syntax=docker/dockerfile:1.7

# ---- builder ----------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

WORKDIR /src

# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
        -trimpath \
        -ldflags="-s -w -X main.Version=${VERSION}" \
        -o /out/modpoll .

# ---- runtime ----------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="modpoll" \
      org.opencontainers.image.description="Modbus polling tool publishing data to NATS" \
      org.opencontainers.image.source="https://github.com/atvirokodosprendimai/go-modpoll"

COPY --from=builder /out/modpoll /modpoll

USER nonroot:nonroot
ENTRYPOINT ["/modpoll"]
