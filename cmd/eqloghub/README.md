# eqloghub (POC)

A minimal HTTP/WebSocket server for receiving batches of normalized outgoing damage events and broadcasting them to subscribers in the same room.

## Run

```bash
go run ./cmd/eqloghub --listen 127.0.0.1:8787
```

## Publish

`roomId` is an arbitrary string. The room token is created on first use and must match for subsequent requests.

```bash
curl -sS -X POST \
  -H "Content-Type: application/json" \
  -H "X-EQLog-Token: secret" \
  http://127.0.0.1:8787/v1/rooms/test/events \
  -d '{
    "publisherId":"demo",
    "sentAtUnixMs": 1730000000000,
    "events": []
  }'
```

## Subscribe (WebSocket)

Connect with the token as a query parameter:

```
ws://127.0.0.1:8787/v1/rooms/test/ws?token=secret
```

The server broadcasts each received publish batch JSON to all current subscribers in the room.
