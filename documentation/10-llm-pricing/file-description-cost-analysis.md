# File Description Pipeline — LLM Cost Analysis

This document provides cost estimates for running the file description pipeline across different LLM providers and models. Use it to select the right model for your deployment based on quality requirements and budget constraints.

## Token Budget Per File

The description pipeline sends a structured context bundle per file and expects a JSON response matching the description schema.

| Component | Estimated Tokens |
|---|---|
| System prompt (role, schema, file role taxonomy) | ~500 |
| File content (truncated to `DESCRIBE_MAX_CONTENT_CHARS`) | ~1,000–3,000 |
| Symbols, imports, exports, consumers context | ~500–1,500 |
| **Total input per file** | **~2,000–5,000** |
| **Output per file** (structured JSON description) | **~300–600** |

Retry overhead: the pipeline retries up to 2× on JSON parse failures. Assuming a ~5% retry rate, add ~5–10% to the total.

## Scaling Reference

| Codebase Size | Input Tokens (est.) | Output Tokens (est.) |
|---|---|---|
| 5,000 files | ~15M–25M | ~2M–3M |
| 10,000 files | ~30M–50M | ~4M–6M |
| 25,000 files | ~75M–125M | ~10M–15M |
| 50,000 files | ~150M–250M | ~20M–30M |
| 100,000 files | ~300M–500M | ~40M–60M |

All estimates below use the **50,000 file midpoint**: 200M input tokens, 25M output tokens.

## Provider Pricing Comparison

*Prices as of March 2026. Verify current rates on provider pricing pages.*

### Anthropic

| Model | Input / 1M tokens | Output / 1M tokens | 50k Files |
|---|---|---|---|
| Claude Opus 4.6 | $15.00 | $75.00 | **$4,875** |
| Claude Sonnet 4.6 | $3.00 | $15.00 | **$975** |
| Claude Haiku 4.5 | $0.80 | $4.00 | **$260** |

### OpenAI

| Model | Input / 1M tokens | Output / 1M tokens | 50k Files |
|---|---|---|---|
| GPT-5.4 | $2.50 | $15.00 | **$875** |
| GPT-5.4 mini | $0.75 | $4.50 | **$263** |
| GPT-5.4 nano | $0.20 | $1.25 | **$71** |
| GPT-4.1 | $2.00 | $8.00 | **$600** |
| GPT-4.1 mini | $0.40 | $1.60 | **$120** |
| GPT-4.1 nano | $0.10 | $0.40 | **$30** |
| GPT-4o | $2.50 | $10.00 | **$750** |
| GPT-4o mini | $0.15 | $0.60 | **$45** |

### Google (Gemini API, Paid Tier)

| Model | Input / 1M tokens | Output / 1M tokens | 50k Files |
|---|---|---|---|
| Gemini 3.1 Pro | $2.00 | $12.00 | **$700** |
| Gemini 2.5 Pro | $1.25 | $10.00 | **$500** |
| Gemini 3 Flash | $0.50 | $3.00 | **$175** |
| Gemini 2.5 Flash | $0.30 | $2.50 | **$123** |
| Gemini 3.1 Flash-Lite | $0.25 | $1.50 | **$88** |
| Gemini 2.5 Flash-Lite | $0.10 | $0.40 | **$30** |

### Google (Gemini Batch API, 50% discount)

| Model | Input / 1M tokens | Output / 1M tokens | 50k Files |
|---|---|---|---|
| Gemini 2.5 Pro Batch | $0.625 | $5.00 | **$250** |
| Gemini 2.5 Flash Batch | $0.15 | $1.25 | **$61** |
| Gemini 2.5 Flash-Lite Batch | $0.05 | $0.20 | **$15** |

### Self-Hosted (Ollama)

| Model | Input / 1M tokens | Output / 1M tokens | 50k Files |
|---|---|---|---|
| Llama 3 / Qwen / Mistral | — | — | **electricity only** |

Self-hosted models have no per-token cost but require GPU hardware. Quality varies significantly by model size. Recommended minimum: 8B parameter model for acceptable description quality; 70B+ for quality comparable to cloud providers.

## Cost Summary (50,000 files, sorted by price)

| Model | Provider | Cost | Tier |
|---|---|---|---|
| Gemini 2.5 Flash-Lite Batch | Google | **$15** | Budget |
| GPT-4.1 nano | OpenAI | **$30** | Budget |
| Gemini 2.5 Flash-Lite | Google | **$30** | Budget |
| GPT-4o mini | OpenAI | **$45** | Budget |
| Gemini 2.5 Flash Batch | Google | **$61** | Budget |
| GPT-5.4 nano | OpenAI | **$71** | Budget |
| Gemini 3.1 Flash-Lite | Google | **$88** | Mid |
| GPT-4.1 mini | OpenAI | **$120** | Mid |
| Gemini 2.5 Flash | Google | **$123** | Mid |
| Gemini 3 Flash | Google | **$175** | Mid |
| Gemini 2.5 Pro Batch | Google | **$250** | Mid |
| Haiku 4.5 | Anthropic | **$260** | Mid |
| GPT-5.4 mini | OpenAI | **$263** | Mid |
| Gemini 2.5 Pro | Google | **$500** | Premium |
| GPT-4.1 | OpenAI | **$600** | Premium |
| Gemini 3.1 Pro | Google | **$700** | Premium |
| GPT-4o | OpenAI | **$750** | Premium |
| GPT-5.4 | OpenAI | **$875** | Premium |
| Sonnet 4.6 | Anthropic | **$975** | Premium |
| Opus 4.6 | Anthropic | **$4,875** | Enterprise |

## Incremental Run Cost

After the initial `describe-full` run, incremental runs (`describe-incremental`) only process changed files and their dependents — typically 5–15% of the codebase.

| Scenario | Files Processed | Cost Range (mid-tier model) |
|---|---|---|
| Small change (1–5 files + dependents) | ~50 | < $1 |
| Feature branch (50–200 files) | ~200–500 | $1–5 |
| Large refactor (10% of codebase) | ~5,000 | $12–25 |
| Major restructure (25% of codebase) | ~12,500 | $30–65 |

Incremental mode uses `content_hash` comparison to skip unchanged files entirely — no LLM or embedding calls are made for skipped files.

## Recommended Strategies

### Mixed-Model Approach

Use different models for different scopes within the same project:

| Scope | Recommended Model | Rationale |
|---|---|---|
| `describe-full` (bulk) | GPT-4.1 nano / Gemini 2.5 Flash-Lite | Cost-efficient for initial bulk generation |
| `describe-incremental` | GPT-4.1 mini / Gemini 2.5 Flash | Better quality for ongoing updates (smaller batch) |
| `describe-file` (user-triggered) | GPT-5.4 / Sonnet 4.6 | Highest quality for on-demand single-file requests |

The pipeline's per-project `llm_provider_config` supports switching models between runs. The `generation_metadata` on each description records which model produced it, and the `/descriptions/summary` endpoint reports `generation_cost_summary` for tracking actual spend.

### Batch API (Google)

Google's Batch API offers 50% cost reduction with higher latency (results within 24 hours). This is ideal for `describe-full` runs where real-time response is not needed. The pipeline would need a batch-aware `Completer` implementation to take advantage of this.

### Caching (OpenAI / Google)

Both OpenAI and Google offer cached input pricing (75–90% discount on repeated prefixes). Since the system prompt and file role taxonomy are identical across all files in a run, caching can significantly reduce input costs:

| Provider | Standard Input | Cached Input | Savings |
|---|---|---|---|
| GPT-5.4 | $2.50 | $0.25 | 90% |
| GPT-5.4 mini | $0.75 | $0.075 | 90% |
| GPT-5.4 nano | $0.20 | $0.02 | 90% |
| Gemini 2.5 Pro | $1.25 | $0.125 | 90% |
| Gemini 2.5 Flash | $0.30 | $0.03 | 90% |

The system prompt (~500 tokens) is a small portion of total input, so effective savings depend on how much of the context is cacheable. Realistic savings: 5–15% on total cost.

## Pricing Sources

- **Anthropic**: https://www.anthropic.com/pricing
- **OpenAI**: https://openai.com/api/pricing/
- **Google**: https://ai.google.dev/gemini-api/docs/pricing
