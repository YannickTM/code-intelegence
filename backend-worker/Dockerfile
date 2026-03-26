# syntax=docker/dockerfile:1.7

FROM golang:1.24-alpine AS builder
WORKDIR /build

RUN apk add --no-cache ca-certificates git gcc g++ musl-dev

# Workspace + module descriptors (cache-friendly layer)
COPY go.work go.work.sum ./
COPY backend-api/go.mod backend-api/go.sum ./backend-api/
COPY backend-worker/go.mod backend-worker/go.sum ./backend-worker/
COPY datastore/go.mod datastore/go.sum ./datastore/
RUN cd backend-worker && go mod download

# Source
COPY backend-worker/ ./backend-worker/
COPY datastore/ ./datastore/

ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -trimpath -ldflags="-s -w -extldflags '-static'" -o /out/backend-worker ./backend-worker/cmd/worker

# --- Runtime ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tini git openssh-client

RUN addgroup -g 1001 -S worker && \
    adduser -u 1001 -S worker -G worker

COPY --from=builder /out/backend-worker /usr/local/bin/backend-worker
RUN chown worker:worker /usr/local/bin/backend-worker

RUN mkdir -p /var/lib/myjungle/repos && chown worker:worker /var/lib/myjungle/repos

USER worker

ENTRYPOINT ["tini", "--", "/usr/local/bin/backend-worker"]
