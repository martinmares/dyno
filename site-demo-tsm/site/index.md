---
title: TSM CETIN Demo
weight: 0
external_refs:
  - id: PRJ1576
    label: "Clooney project: Platformizace TSM"
    url: "https://clooney.cetin/spaces/cetin?entity=projects&guid=1576"
    type: clooney_project
---

# TSM CETIN Demo

Tahle ukazkova site je maly vyrez z externiho markdown repozitare
`.tmp/tsm-cetin-docs-main`. Cilem je ukazat, jak by mela vypadat dokumentace po
regenerovani s normalizovanym polem `external_refs`.

Puvodni domenovy frontmatter zustava zachovany, ale Dyno ma pro UI jednotny
kontrakt:

```yaml
external_refs:
  - id: REQ0124546
    label: "Clooney: DB PostgreSQL - rozdeleni"
    url: "https://clooney.cetin/spaces/cetin?entity=requirements&guid=124546"
    type: clooney_requirement
```

## Ukazky

| Stranka | Co ukazuje |
|---|---|
| [BRD pozadavek](detail-design/brd.md) | Clooney requirement + project vazba |
| [Infra skupina](detail-design/infra/index.md) | Clooney group + parent/root vazby |
| [DB PostgreSQL](detail-design/infra/db-postgresql.md) | Konkretni requirement se skupinou a komponentami |
| [Ticket typ](model/ticket/tech-poruchovy-aplikace.md) | TSM formular + TSM proces |
| [Ticket model diagram](model/ticket/base-classes.md) | Mermaid `classDiagram` z modeloveho pohledu |

## Doporučení pro generator

- Zachovat existujici metadata jako `refid`, `clooney_url`, `project_refid`,
  `ticket_type`, `ui_forms` a `process_refs`.
- Navic generovat `external_refs` jako plochy seznam klikatelnych vazeb.
- Do `id` davat stabilni technicke ID, do `label` lidsky popis a do `type`
  strojove citelny typ vazby.
