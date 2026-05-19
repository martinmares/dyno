---
kind: class
id: ticket.ticket.tech_poruchovy_aplikace
name: TicketTechPoruchovyAplikace
label: Technologicky poruchovy na aplikaci
domain: TICKET
status: active
owner: ticket
class_role: logical_specialization
stereotype: TicketType
extends: ticket.ticket
entity_type: Ticket
reverse_engineered_at: "2026-05-05T12:10:57+02:00"
description: Cetin ticket type `Tech.Poruchovy.Aplikace` nad core entitou Ticket.
specification:
  code: Tech.Poruchovy.Aplikace
  name: Technologicky poruchovy na aplikaci
  entity_type: Ticket
ticket_type:
  id: b1a44470-d76c-4191-97ff-d1640e688797
  code: Tech.Poruchovy.Aplikace
  name: Technologicky poruchovy na aplikaci
  valid: true
  entity_specification_code: Ticketing.Tickets.TL
  default_process: Tick-Main
  mnemonic_code: Ticketing.Tickets.TL
  sla: Ticket.TL
  create_privilege: Tick.Ticket.Create.TL.Poruchovy.Aplikace
ui_forms:
  - slot: tsm-ticket-detail
    form_code: Ticket.Detail.TL
    form_url: https://tsm-test.cetin/config/forms/a375b67a-dc4e-436f-9028-5e04a660b447
  - slot: tsm-ticket-new
    form_code: docasneZalozeniTicketu
    form_url: https://tsm-test.cetin/config/forms/0cd2693b-0620-4048-b17b-2b63ba8eddc2
process_refs:
  - process_code: Tick-Main
    process_name: Poruchove listky - HLAVNI PROCES
    process_url: https://tsm-test.cetin/process/process-definition/fc1438d4-3573-4b68-9d3d-bcb8295a7213
    process_type: Ticketing
    state: DRAFT
    version: "5.4"
    deployed: true
    relation: default process from TicketType.defaultProcessExpr
external_refs:
  - id: Ticket.Detail.TL
    label: "Form: Ticket.Detail.TL"
    url: "https://tsm-test.cetin/config/forms/a375b67a-dc4e-436f-9028-5e04a660b447"
    type: tsm_form
  - id: docasneZalozeniTicketu
    label: "Form: docasneZalozeniTicketu"
    url: "https://tsm-test.cetin/config/forms/0cd2693b-0620-4048-b17b-2b63ba8eddc2"
    type: tsm_form
  - id: Tick-Main
    label: "Process: Poruchove listky - HLAVNI PROCES"
    url: "https://tsm-test.cetin/process/process-definition/fc1438d4-3573-4b68-9d3d-bcb8295a7213"
    type: tsm_process
---

# TicketTechPoruchovyAplikace

Technologický poruchový ticket nad aplikací. Stránka ukazuje, že původní
doménový frontmatter může zůstat bohatý a zároveň lze nad ním vygenerovat
jednoduché `external_refs` pro UI.

## Specifikace

| Pole | Hodnota |
|---|---|
| Ticket type | `Tech.Poruchovy.Aplikace` |
| Entity type | `Ticket` |
| Default process | `Tick-Main` |
| Mnemonic | `Ticketing.Tickets.TL` |
| SLA | `Ticket.TL` |

## UI formuláře

- `Ticket.Detail.TL` - detail ticketu
- `docasneZalozeniTicketu` - založení ticketu

## Procesní vazby

- `Tick-Main` - Poruchové lístky, hlavní proces
