# Phase 2 — Kubernetes Scaling

## Goal

Move from Docker Compose (always-on containers) to Kubernetes with demand-driven scaling. Workers should only run while indexing jobs are active. The backend API and infrastructure services remain permanently available.

## Current State (Docker Compose)

All application services run permanently with `restart: unless-stopped`:

| Service        | Running Model    | Needed Permanently? |
| -------------- | ---------------- | ------------------- |
| backend-api    | always-on        | yes                 |
| backend-worker | always-on (idle) | no                  |
| mcp-server     | always-on        | yes                 |
| backoffice     | always-on        | yes                 |
| postgres       | always-on        | yes                 |
| qdrant         | always-on        | yes                 |
| redis          | always-on        | yes                 |

The worker currently idles in skeleton mode, logging heartbeats every 30 seconds. It consumes resources without producing value between indexing runs.

## Design Principles

- Scale indexing workloads to zero when idle
- Limit to one worker per project to avoid conflicting git and write operations
- Keep the API layer always available for queries and MCP requests
- Preserve the existing Redis-based job queue and pub/sub architecture

## Service Scaling Strategy

### Backend API — Always-On with HPA

The API serves HTTP requests from the backoffice, MCP server, and query traffic. It must remain available at all times.

Scaling model:

- Kubernetes Deployment with `minReplicas: 1`
- HorizontalPodAutoscaler based on CPU/request rate
- Liveness probe: `GET /health/live`
- Readiness probe: `GET /health/ready`
- Graceful shutdown: 10 seconds (already implemented)

No changes to application code required.

### Backend Worker — Scale-to-Zero with KEDA

The worker only needs to run while indexing jobs are pending or in progress. Between indexing runs it should scale to zero.

The worker is a single self-contained binary with the go-tree-sitter parser embedded.

Scaling model:

- Kubernetes Deployment managed by KEDA ScaledObject
- Scale trigger: Redis list length on asynq queues (pending job count)
- Scale range: `minReplicaCount: 0`, `maxReplicaCount: N` (one per concurrent project)
- Cooldown: scale to zero after a configurable idle period (e.g. 5 minutes after last job completes)

KEDA trigger configuration (conceptual):

```yaml
triggers:
  - type: redis
    metadata:
      address: redis:6379
      listName: "asynq:{default}:pending"
      listLength: "1"
      activationListLength: "1"
```

One-worker-per-project constraint:

- The asynq uniqueness window (1 hour per project+job_type) already prevents duplicate jobs for the same project
- Worker concurrency is configured in the application (`jobs.worker_concurrency: 4`), meaning one worker pod can process up to 4 jobs from different projects concurrently
- For strict single-project isolation, reduce `worker_concurrency` to 1 and scale `maxReplicaCount` to the number of projects that may index concurrently
- Alternatively, keep concurrency at 4 and let one worker pod handle multiple projects — the uniqueness window prevents conflicting operations on the same project

Pod spec (conceptual):

```yaml
spec:
  containers:
    - name: backend-worker
      image: myjungle/backend-worker
      resources:
        requests:
          cpu: 500m
          memory: 512Mi
        limits:
          cpu: 2000m
          memory: 2Gi
```

### MCP Server and Backoffice — Always-On

Both services serve user-facing traffic and should remain available:

- MCP server: Deployment with `replicas: 1` (HPA optional based on agent load)
- Backoffice: Deployment with `replicas: 1` (static SPA serving, minimal resource usage)

### Infrastructure Services

For Kubernetes deployments, infrastructure services can either run as StatefulSets within the cluster or use managed services:

| Service    | Self-Hosted Option        | Managed Alternative               |
| ---------- | ------------------------- | --------------------------------- |
| PostgreSQL | StatefulSet + PVC         | Cloud SQL, RDS, Aurora            |
| Qdrant     | StatefulSet + PVC         | Qdrant Cloud                      |
| Redis      | StatefulSet or Deployment | ElastiCache, Memorystore, Upstash |

Managed services are recommended for production to reduce operational burden. Self-hosted StatefulSets are fine for staging or cost-sensitive environments.

## Resource Profiles

Suggested resource requests and limits per container:

| Container      | CPU Request | CPU Limit | Memory Request | Memory Limit |
| -------------- | ----------- | --------- | -------------- | ------------ |
| backend-api    | 100m        | 500m      | 128Mi          | 512Mi        |
| backend-worker | 500m        | 2000m     | 512Mi          | 2Gi          |
| mcp-server     | 50m         | 200m      | 64Mi           | 256Mi        |
| backoffice     | 10m         | 100m      | 32Mi           | 128Mi        |

The worker resource profile includes the embedded go-tree-sitter parser overhead (grammar loading, AST construction). Memory usage scales with the number of supported language grammars (28 languages) and concurrent file parsing. Adjust after profiling real workloads.

## Shared Storage

The worker requires access to a repository cache volume (`/var/lib/myjungle/repos`). In Kubernetes:

- Use a PersistentVolumeClaim with `ReadWriteMany` access mode if multiple worker pods may run concurrently
- Alternatively, use `ReadWriteOnce` if worker concurrency is limited to one pod at a time
- Consider ephemeral storage if full clones per job are acceptable (simplifies volume management at the cost of clone time)

## Configuration Changes

Environment variable adjustments for Kubernetes:

| Variable       | Docker Compose Value          | Kubernetes Value                 |
| -------------- | ----------------------------- | -------------------------------- |
| `POSTGRES_DSN` | `postgres://app:app@postgres` | cluster-internal or managed DNS  |
| `REDIS_URL`    | `redis://redis:6379/0`        | cluster-internal or managed DNS  |
| `QDRANT_URL`   | `http://qdrant:6333`          | cluster-internal or managed DNS  |
| `OLLAMA_URL`   | `host.docker.internal:11434`  | cluster-internal or external URL |

Secrets (`SSH_KEY_ENCRYPTION_SECRET`, database credentials) should be stored in Kubernetes Secrets and mounted as environment variables.

Note: No separate parser address configuration is needed — the parser is compiled into the worker binary and runs in-process.

## Migration Path

1. Containerize for Kubernetes: existing Docker images work as-is (distroless/alpine, non-root, signal handling)
2. Create Kubernetes manifests (Deployments, Services, ConfigMaps, Secrets)
3. Deploy infrastructure services (or configure managed alternatives)
4. Deploy always-on services (backend-api, mcp-server, backoffice)
5. Install KEDA in the cluster
6. Deploy worker ScaledObject
7. Validate scale-to-zero behavior and job processing
8. Remove Docker Compose `--profile app` workflow for production

## Observability in Kubernetes

- Worker pod lifecycle events (scale-up, scale-down) should be logged for debugging job delays
- KEDA scaler metrics should be exposed to Prometheus for queue depth monitoring
- Pod startup time matters: if cold-start latency is too high, consider `minReplicaCount: 1` during business hours
- Monitor worker memory usage under load — embedded parser memory scales with concurrent file count and grammar complexity

## Future Considerations

- **Node affinity**: worker pods with large repository clones may benefit from node pools with fast SSD storage
- **Spot/preemptible instances**: worker pods are good candidates for spot instances since indexing jobs are retryable
- **Multi-region**: Qdrant and PostgreSQL replication for read-heavy query traffic across regions
- **Ollama scaling**: GPU-aware scheduling for Ollama pods if embedding becomes a bottleneck

## Open Questions

- Should KEDA scale based on asynq pending count, or should the backend API expose a custom metric (e.g. "projects with pending jobs")?
- What is an acceptable cold-start latency for the worker pod (grammar loading happens at startup)?
- Should the repository cache volume be shared across worker pods or ephemeral per pod?
