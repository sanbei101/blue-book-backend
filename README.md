# Blue Book Backend

## AI and CLI access

Users create a delegated API Key while signed in with their normal JWT:

```sh
curl -X POST http://localhost:8080/api/v1/auth/api-keys \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"my-agent"}'
```

The plaintext key has the `bbk_` prefix and is returned only by that response.
Store it in a secret manager or environment variable. API keys can perform normal
social actions, but cannot change passwords, log out sessions, or manage API keys.
List and revoke keys through the same JWT-authenticated endpoints:

```sh
curl -H "Authorization: Bearer $ACCESS_TOKEN" http://localhost:8080/api/v1/auth/api-keys
curl -X DELETE -H "Authorization: Bearer $ACCESS_TOKEN" http://localhost:8080/api/v1/auth/api-keys/$KEY_ID
```

Build or install the CLI from this repository:

```sh
go install ./cmd/bluebook
export BLUEBOOK_API_KEY='bbk_...'
export BLUEBOOK_API_URL='https://bluebook.example.com/api/v1'

bluebook post create --title "A post" --content "Published by my agent" --tag ai
bluebook comment create --post POST_ID --content "A comment"
bluebook media upload image.png
bluebook api get /me/collections
```

`bluebook` writes JSON to stdout. `bluebook api <method> <path> [--data JSON]`
is a forward-compatible escape hatch for every existing or future user API.

## Database update

New databases use `sqlc/schema.sql`. Apply
`sqlc/migrations/20260812_add_api_keys.sql` once to add API Key support to an
existing database.
