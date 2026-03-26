# PostgreSQL Implementation Assets

## Layout

- `migrations/`: schema migrations (`golang-migrate` format)
- `queries/`: `sqlc` query sources
- `sqlc/`: generated Go code output target

## Generate sqlc Code

```bash
sqlc generate
```

If `sqlc` is not installed locally, use a containerized run:

```bash
docker run --rm \
  -v "$(pwd)":/src \
  -w /src \
  sqlc/sqlc:1.28.0 generate
```
