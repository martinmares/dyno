---
kind: model-view
id: view.ticket.base.demo
name: TICKET Base Classes
title: "Ticket model - Mermaid class diagram"
view_type: class-diagram
domain: ticket
status: draft
classes:
  - ticket.ticket
  - ticket.ticket_type
  - ticket.priority
  - ticket.severity
  - ticket.resolution
  - ticket.related_party
  - ticket.related_entity
external_refs:
  - id: view.ticket.base
    label: "Source view: TICKET Base Classes"
    url: "https://tsm-test.cetin/model/ticket/_Views/base-entities"
    type: tsm_model_view
  - id: ticket.ticket.tech_poruchovy_aplikace
    label: "Example ticket type"
    url: "../ticket/tech-poruchovy-aplikace.md"
    type: dyno_page
---

# Ticket model - Mermaid class diagram

Tahle stránka je migrovaná z modelového pohledu
`model/ticket/_Views/base-entities.md`. Původní diagram je výrazně větší, pro
demo site je zkrácený na hlavní vazby okolo ticketu.

```mermaid
%% @datamodel.view id=view.ticket.base.demo mode=enhanced
classDiagram
  %% @datamodel.ref node=ticket_ticket class=ticket.ticket display=keys
  class ticket_ticket["Ticket"]

  %% @datamodel.ref node=ticket_ticket_type class=ticket.ticket_type display=keys
  class ticket_ticket_type["TicketType"]

  %% @datamodel.ref node=ticket_priority class=ticket.priority display=keys
  class ticket_priority["Priority"]

  %% @datamodel.ref node=ticket_severity class=ticket.severity display=keys
  class ticket_severity["Severity"]

  %% @datamodel.ref node=ticket_resolution class=ticket.resolution display=keys
  class ticket_resolution["Resolution"]

  %% @datamodel.ref node=ticket_related_party class=ticket.related_party display=keys
  class ticket_related_party["RelatedParty"]

  %% @datamodel.ref node=ticket_related_entity class=ticket.related_entity display=keys
  class ticket_related_entity["RelatedEntity"]

  ticket_ticket --> ticket_ticket_type : type
  ticket_ticket --> ticket_priority : priority
  ticket_ticket --> ticket_severity : severity
  ticket_ticket --> ticket_resolution : resolution
  ticket_ticket --> ticket_related_party : parties
  ticket_related_entity --> ticket_ticket : references
```

## Poznámka ke kompatibilitě

Externí repo používá standardní Mermaid zápis:

````markdown
```mermaid
classDiagram
  Ticket --> TicketType
```
````

To je přesně formát, který Dyno podporuje. Renderer fence převede na
`<pre class="mermaid">...</pre>` a frontend ho následně vykreslí přes lokální
`assets/mermaid.min.js`.
