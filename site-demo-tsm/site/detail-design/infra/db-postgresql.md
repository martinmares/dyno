---
refid: REQ0124546
title: "DB PostgreSQL - rozdeleni"
clooney_url: "https://clooney.cetin/spaces/cetin?entity=requirements&guid=124546"
source: clooney-index
snapshot_tag: PRJ1576-initial-20260505-https
sync_run_at: 2026-05-05T08:12:08Z
project_refid: PRJ1576
project_title: "Platformizace TSM"
wp_refid: WP4238
wp_title: "Faze TC a HLD"
parent_refid: RG0124422
parent_title: "Infra - Domenovy model na navaznych technologiich"
root_refid: REQ0122859
root_title: "Uzivatelske pozadavky (BRD)"
guid: 124546
kind: 2
lcstage: 1
lcstep: 5
priority: 5
moscow: 5
ordering: 5
components:
  - Operations-Infra
  - TSM-Core
group_refid: RG0124422
group_title: "Infra - Domenovy model na navaznych technologiich"
external_refs:
  - id: REQ0124546
    label: "Clooney: DB PostgreSQL - rozdeleni"
    url: "https://clooney.cetin/spaces/cetin?entity=requirements&guid=124546"
    type: clooney_requirement
  - id: RG0124422
    label: "Group: Infra - Domenovy model"
    url: "https://clooney.cetin/spaces/cetin?entity=requirements&guid=124422"
    type: clooney_group
  - id: WP4238
    label: "Work package: Faze TC a HLD"
    url: "https://clooney.cetin/spaces/cetin?entity=workpackages&refid=WP4238"
    type: clooney_work_package
---

# REQ0124546 - DB PostgreSQL - rozdělení

## Vstup od zákazníka

Navrhnout a realizovat rozdělení databáze Postgre do více instancí.
Předpokládáme dodržení těchto principů a vyjasnění otázek:

- Každá BD bude využívat pro své interní doménové potřeby svou instanci
  Postgre. Vnitřní rozložení dat v databázi je na zodpovědnosti BD.
- Pro každou doménu bude existovat jedna samostatná Postgres DB.
- Přístup do databáze z jiné než mateřské domény je obecně nepřípustný.
- Databáze bude vytvořena Infra oddělením CETIN dle standardů CETIN.

Zajistěte migraci dat a podporu nasazení na všech prostředích.

## Naše zpracování

### Kontext a rozsah

- Doplnit kontext a vymezení požadavku.

### Návrh řešení

- Doplnit detailní návrh řešení.

### Dopady

- Doplnit dopady na konfigurace, microservices, data, provoz, CI/CD,
  bezpečnost a dokumentaci podle povahy požadavku.

## Vazby

- Skupina: `RG0124422` - Infra - Doménový model na návazných technologiích
- Komponenty: `Operations-Infra`, `TSM-Core`
