---
title: API Widget
description: Interactive REST API calls directly in documentation
weight: 40
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

## Jednoduché GET

```api
GET https://httpbin.org/get
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

> [!TIP]
> Proxy běží server-side, takže CORS není problém. Funguje i pro lokální API (`http://localhost:8080`).

> [!NOTE]
> Odpovědi jsou omezeny na 1 MB. JSON se automaticky odsadí pro čitelnost.
