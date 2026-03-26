#!/usr/bin/env bash
#
# deploy-script.sh — Automated deployment pipeline for the web service.
# Usage: ./deploy-script.sh [staging|production]
#

set -euo pipefail

. ./lib/logging.sh
source ./lib/config.sh

DEPLOY_ENV="${1:-staging}"
DEPLOY_TAG="$(git rev-parse --short HEAD)"
REGISTRY="ghcr.io/acme/web-service"
IMAGE="${REGISTRY}:${DEPLOY_TAG}"

# Log a timestamped message to stdout.
log_info() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] INFO: $*"
}

# Log an error message and exit.
log_error() {
    echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] ERROR: $*" >&2
    exit 1
}

# Build the Docker image with the current git tag.
build_image() {
    log_info "Building image ${IMAGE}"
    docker build \
        --build-arg "GIT_SHA=${DEPLOY_TAG}" \
        --build-arg "BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        -t "${IMAGE}" \
        -f docker/Dockerfile .
}

# Push the image to the container registry.
push_image() {
    log_info "Pushing image to registry"
    docker push "${IMAGE}"
}

# Run database migrations before deployment.
run_migrations() {
    local db_url
    db_url="$(get_config "${DEPLOY_ENV}" "database_url")"
    log_info "Running migrations against ${DEPLOY_ENV}"
    ./bin/migrate --url "${db_url}" --direction up
}

# Deploy to the target Kubernetes cluster.
deploy_to_cluster() {
    local namespace="web-${DEPLOY_ENV}"
    log_info "Deploying to namespace ${namespace}"

    kubectl set image \
        "deployment/web-service" \
        "web-service=${IMAGE}" \
        --namespace "${namespace}"

    kubectl rollout status \
        "deployment/web-service" \
        --namespace "${namespace}" \
        --timeout=300s
}

# Run a quick smoke test against the deployed service.
run_smoke_test() {
    local base_url
    base_url="$(get_config "${DEPLOY_ENV}" "base_url")"
    log_info "Running smoke test against ${base_url}"

    local status_code
    status_code=$(curl -s -o /dev/null -w '%{http_code}' "${base_url}/health")
    if [[ "${status_code}" != "200" ]]; then
        log_error "Smoke test failed: health check returned ${status_code}"
    fi
    log_info "Smoke test passed"
}

# --- Main ---
main() {
    log_info "Starting deployment for env=${DEPLOY_ENV} tag=${DEPLOY_TAG}"
    build_image
    push_image
    run_migrations
    deploy_to_cluster
    run_smoke_test
    log_info "Deployment complete"
}

main "$@"
