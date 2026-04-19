---
title: DevOps Handbook
weight: 0
---

# DevOps Handbook

Practical recipes for Docker, CI/CD pipelines, Kubernetes and observability. Designed for small-to-medium engineering teams who want to ship reliably without a dedicated platform team.

> [!NOTE]
> Examples use GitHub Actions for CI/CD. The concepts apply equally to GitLab CI, CircleCI or Buildkite.

## Sections

| Section | Topics |
|---|---|
| [Docker](docker/) | Multi-stage builds, image hygiene, Compose |
| [CI/CD](ci-cd/) | GitHub Actions, caching, release automation |
| [Kubernetes](kubernetes/) | Deployments, health checks, resource limits |
| [Monitoring](monitoring/) | Prometheus, Grafana, alerting basics |

## Core principles

- **Immutable artifacts** — build once, deploy the same image everywhere.
- **Everything as code** — infra, pipelines, dashboards in version control.
- **Ship small** — small PRs + frequent deploys = less blast radius.
- **Observability first** — if you can't measure it, you can't fix it.
