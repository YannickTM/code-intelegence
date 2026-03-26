# syntax=docker/dockerfile:1.7

FROM golang:1.24-alpine AS builder
WORKDIR /build

RUN apk add --no-cache ca-certificates git

# Workspace + module descriptors (cache-friendly layer)
COPY go.work go.work.sum ./
COPY backend-api/go.mod backend-api/go.sum ./backend-api/
COPY backend-worker/go.mod backend-worker/go.sum ./backend-worker/
COPY datastore/go.mod datastore/go.sum ./datastore/
RUN cd backend-api && go mod download

# Source
COPY backend-api/ ./backend-api/
COPY datastore/ ./datastore/

ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
  go build -trimpath -ldflags="-s -w" -o /out/backend ./backend-api/cmd/api

FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates wget

COPY --from=builder /out/backend /usr/local/bin/backend

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/backend"]
