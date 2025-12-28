# Build the manager binary
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY Makefile Makefile
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the remaining Go source (relies on .dockerignore to filter)
COPY . .

# Build
RUN --mount=type=cache,target=/go/pkg/mod,rw \
    --mount=type=cache,target=/root/.cache/go-build,rw \
    CGO_ENABLED=0 make -j build TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH}

FROM alpine:latest
WORKDIR /
COPY --from=builder /workspace/bin/manager /manager
COPY --from=builder /workspace/bin/stats /stats
USER 65532:65532

ENTRYPOINT ["/manager"]
