---
title: Environment Placeholders
description: Using .env and process environment values inside Markdown
weight: 45
---

# Environment Placeholders

dyno umí před samotným Markdown renderem nahradit placeholdery napsané VELKÝMI PÍSMENY.

Typické použití:

- přepínání hostname mezi dev/staging/prod
- sdílená dokumentace nasazovaná do různých prostředí
- ukázkové API widgety bez natvrdo zapsaných URL

## Odkud se hodnoty berou

Zdrojové vrstvy jsou:

- `./.env`
- `./site/.env`
- environment proměnné procesu

Proměnné procesu mají nejvyšší prioritu.

## Markdown příklad

```markdown
Base URL: {{HTTPBIN_URL}}

```api
{{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/get
Accept: application/json
```
```

## Ukázka `.env`

```env
HTTP_METHOD_FOR_GET=GET
HTTPBIN_URL=https://httpbin.org
```

## Živý příklad

```api
{{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/get
Accept: application/json
```

## Co se nedosazuje

Placeholdery jako `{{token}}`, `{{userId}}` nebo `{{projectSlug}}` zůstávají beze změny.
To je záměr: API widget je pak ukáže jako editovatelná pole v UI.

```api
{{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/anything/{{userId}}
Authorization: Bearer {{token}}
```

V tomhle příkladu:

- `{{HTTP_METHOD_FOR_GET}}` a `{{HTTPBIN_URL}}` se dosadí před renderem
- `{{userId}}` a `{{token}}` zůstanou interaktivní
