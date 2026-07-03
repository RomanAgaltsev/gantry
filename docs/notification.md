# Notifications

gantry can push events to one or more channels. Notifications are **best-effort**: a failed
send is logged and never fails a deploy, promote, rollback, or drift check.

## Events

`deployed` · `promoted` · `rolled_back` · `verify_failed` · `drift_alarm`

Each channel may subscribe to a subset with `events:`; omit it to receive all kinds.

## Backends

### webhook (Telegram-compatible)

POSTs JSON `{ "chat_id"?, "text", "event", "environment", "commit", "by", "timestamp" }`. A
Telegram Bot API `sendMessage` URL uses `chat_id` + `text`; a generic sink reads the structured
fields.

```yaml
notifications:
  - kind: webhook
    url: ${env:GANTRY_WEBHOOK_URL}          # https://api.telegram.org/bot<token>/sendMessage
    chat_id: ${env:GANTRY_TELEGRAM_CHAT_ID} # optional
    events: [deployed, promoted, rolled_back, verify_failed, drift_alarm]
```

### email

```yaml
  - kind: email
    smtp: { host: smtp.example.com, port: 587, username: ops, password: ${file:/run/secrets/smtp} }
    from: gantry@example.com
    to: [ops@example.com]
    events: [verify_failed, drift_alarm]
```

Messages are fixed, single-line, e.g. `deployed 3 pin(s) to test`,
`promoted test@1a2b3c4 -> prod (3 pins)`, `verify failed for prod`.
