---
title: API Widget
description: Interactive REST API calls directly in documentation
weight: 40
external_refs:
  - id: API-101
    label: HTTPBin test API
    url: https://httpbin.org
    type: api
  - id: SEC-014
    label: Proxy allowlist policy
    url: https://github.com/mares/dyno/blob/main/README.md#configuration
    type: security
---

# API Widget

Dokumentace může obsahovat živé REST API volání — čtenář si je může rovnou vyzkoušet přímo v prohlížeči, bez nutnosti otevírat Postman nebo terminál.

## Syntaxe

````markdown
```api
GET https://api.example.com/endpoint
Header-Name: value
```
````

První řádek je vždy `METHOD URL`. Další řádky jsou volitelné HTTP hlavičky (`Klíč: hodnota`).

Pro interní endpoint se self-signed TLS certifikátem použij explicitní variantu `api-insecure`. Vypnutí kontroly certifikátu platí pouze pro daný widget:

````markdown
```api-insecure no-auth
GET https://internal-api.example.test/status
```
````

Volitelný příznak `no-auth` odstraní z widgetu celou sekci Auth. Lze jej použít také jako ```` ```api no-auth ```` bez vypnutí TLS kontroly.

## Dynamicky nalezené requesty

Odpověď service discovery může pomocí JSONPath vytvořit další volatelné API widgety:

````markdown
```api-insecure no-auth
GET {{TSM_BASE_URL}}{{GENERATE_CONFIG_PATH}}/tsm-ticket
@follow-jsonpath: $[*].labels.__metrics_path__
@follow-method: GET
```
````

Dyno podporuje vlastnosti, indexy polí, vlastnosti v uvozovkách a wildcard `[*]`. Nalezené cesty s úvodním `/` i bez něj skládá od originu rodičovského requestu. Absolutní HTTP(S) URL zachová. Odstraní duplicity, zobrazí nejvýše 50 widgetů a navazující requesty nespouští automaticky.

## Jednoduché GET

```api
GET {{HTTPBIN_URL}}/get
```

## GET s hlavičkami

```api
GET https://httpbin.org/headers
X-Custom-Header: dyno-docs
Accept: application/json
```

## POST s tělem

```api
POST https://httpbin.org/post
Content-Type: application/json
```

## Proměnné

Použij `{{jméno}}` pro editovatelná pole — ideální pro tokeny nebo ID:

```api
GET https://httpbin.org/bearer
Authorization: Bearer {{token}}
```

```api
GET https://httpbin.org/anything/{{userId}}
Accept: application/json
```

## Placeholdery z prostředí

Když použiješ placeholder napsaný VELKÝMI PÍSMENY, dyno ho před renderem dosadí z:

- `./.env`
- `./site/.env`
- environment proměnných procesu

Environment procesu má nejvyšší prioritu. To je praktické třeba pro nasazení v PODu nebo přes CI/CD.

```api
{{HTTP_METHOD_FOR_GET}} {{HTTPBIN_URL}}/get
```

Například v `site/.env` může být:

```env
HTTP_METHOD_FOR_GET=GET
HTTPBIN_URL=https://httpbin.org
```

Po renderu z toho vznikne normální API widget s konkrétní URL.

Naopak placeholdery jako `{{token}}` nebo `{{userId}}` zůstávají interaktivní a čtenář je vyplňuje až v UI widgetu.

> [!TIP]
> Proxy běží server-side, takže CORS není problém. Funguje i pro lokální API (`http://localhost:8080`).

> [!NOTE]
> Odpovědi jsou omezeny na 1 MB. JSON se automaticky odsadí pro čitelnost.
