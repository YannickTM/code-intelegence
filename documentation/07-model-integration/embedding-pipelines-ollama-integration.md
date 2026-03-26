<details>
<summary>Relevant source files</summary>

The following files were used as context for generating this wiki page:

- [concept/tickets/backend-api/08-provider-settings.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/tickets/backend-api/08-provider-settings.md)
- [concept/01-system-overview.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/01-system-overview.md)
- [concept/tickets/backend-worker/01-foundation.md](https://github.com/YannickTM/code-intelegence/blob/main/concept/tickets/backend-worker/01-foundation.md)
</details>

# Embedding Pipelines (Ollama Integration)

## Introduction

The Embedding Pipeline is a core component of the MYJUNGLE platform, responsible for transforming raw code chunks into semantic vectors. These vectors enable high-quality code retrieval for AI coding agents. The system utilizes a self-hosted **Ollama** instance as its primary embedding provider, ensuring that source code remains within the user's infrastructure to satisfy security and privacy requirements.

The pipeline operates across two primary tiers: a global server default and per-project overrides. This architecture allows for centralized management while providing the flexibility for individual projects to use specialized models or different dimensions. The integration supports both batch processing during initial repository indexing and single-query embedding during real-time searches.

Sources: [concept/tickets/backend-api/08-provider-settings.md](), [concept/01-system-overview.md:10-15]()
