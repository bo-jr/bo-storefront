# bo-storefront

Source for the `storefront` service. **Source only** — no manifests live here.

Canonical spec: [`bo-platform/docs/BUILD-PLAN.md`](https://github.com/bo-jr/bo-platform/blob/main/docs/BUILD-PLAN.md)
§4 (layout), Phase 2 (the service), Phase 6 (the milestone).
Read the relevant phase before writing anything.

## What this service is

`GET /checkout?sku=X&qty=N` — fans out to `catalog` and `pricing`, assembles a quote.

**This is the canary target.** The entire lab exists to demonstrate one thing: deploying
`storefront` v2 with `FAILURE_RATE=0.05`, watching Argo Rollouts shift 10% of traffic,
watching the burn-rate AnalysisTemplate abort it, and watching traffic return to v1 with
zero human input. Everything else is scaffolding for that.

## The name

**The `bo-` prefix stops at the repo boundary.** Inside the cluster this service is
`storefront` — Rollout, Service, `app` label, Prometheus `service=` label, SLO name,
Discord message. Never `bo-storefront`.

The sole exception is the image, because `ghcr.io/${{ github.repository }}` resolves to
`ghcr.io/bo-jr/bo-storefront` and fighting that default is not worth it. So the chart
takes `image.repository` **with** the prefix and `name` **without**.

## Required of every service, without exception

- `GET /healthz` (liveness), `GET /readyz` (checks downstream deps)
- `GET /metrics` — `http_requests_total{service,route,status,version}` and
  `http_request_duration_seconds` histogram, same labels
- OTel tracing, W3C traceparent propagation, OTLP export to the local Alloy
- Structured JSON logs to stdout including `trace_id`
- Graceful shutdown on SIGTERM with connection draining

Shared behaviour comes from `bo-service-kit`. If you are writing telemetry or chaos code
in this repo, it belongs in the kit instead.

## Chaos knobs

Read at startup, stamped into a `version` label:

| Var | Effect |
|---|---|
| `FAILURE_RATE` | float 0.0–1.0; that fraction of requests return 500 |
| `EXTRA_LATENCY_MS` | int; sleep injected before responding |
| `APP_VERSION` | string; must appear as a Prometheus label and a pod label |

## What lives here

```
cmd/storefront/main.go
Dockerfile                    # multi-arch, built natively per arch
chart-values.yaml             # values for the shared chart
.github/workflows/ci.yml      # calls the reusable workflow
```

`chart-values.yaml` declares `dependencies: [catalog, pricing]`. The chart generates the
Istio `AuthorizationPolicy`, the `readyz` dependency checks, and the ServiceMonitor from
that list. Under default-deny, forgetting a dependency is an outage — declare them.

Pin `bo-service-chart` **by exact version**. An unpinned chart edit silently changes all
three services' manifests at once.

## What must never live here

- Rendered manifests — those are CI output, committed to `bo-deploy`
- A `manifests/` or `base/` directory — the shared chart replaces it
- Kustomize anything — one templating tool, not two

## Metrics discipline

**Never label a Prometheus metric with a commit SHA, image digest, or Rollout hash.**
Unbounded cardinality; it will quietly consume the whole store. Those belong in GitHub
Deployments and Discord messages, where cardinality is free.
## Non-negotiable (inherited from `bo-platform/CLAUDE.md`)

- **No floating tags. Ever.** Not `latest`, `lts`, `stable`, or partial semver (`:1`,
  `:1.2`). Images pinned by **manifest-list digest**, charts by exact semver.
- **Pin the index digest, never a per-arch digest.** A platform-specific digest pulls
  fine on one machine and fails `no match for platform` on the other. This is the most
  likely portability bug in the lab.
- **Cross-platform, always.** Everything must work on `darwin/arm64` (MacBook, the
  runtime target) and `linux/amd64` (Windows/WSL2, build and test only). Images build
  `linux/amd64,linux/arm64`.
- **LF line endings**, enforced by `.gitattributes`. A CRLF `.sh` inside a Linux image
  fails as `bad interpreter: /bin/bash^M`.
- **When something fails, check architecture first** — the usual cause of
  `ImagePullBackOff` and `exec format error` here.
- If reality contradicts the plan, **stop and say so.** Do not improvise around it;
  record the outcome in `bo-platform/docs/DECISIONS.md`.
