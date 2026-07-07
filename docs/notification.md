# Notifications

gantry can push events to one or more channels. Notifications are **best-effort**: a failed
send is logged and never fails a deploy, promote, rollback, or drift check.

## Events

`deployed` · `promoted` · `rolled_back` · `verify_failed` · `drift_alarm`

Each channel may subscribe to a subset with `events:`; omit it to receive all kinds.

## Backends

### webhook / slack / telegram

`kind: webhook`, `kind: slack`, and `kind: telegram` are thin wrappers over one webhook
core: gantry POSTs JSON to `url`. They differ only in the payload shape they send:

| kind | payload | notes |
| --- | --- | --- |
| `webhook` | `{ "text", "event", "environment", "commit", "by", "timestamp" }` | The generic structured payload a custom sink reads. |
| `slack` | `{ "text": … }` | The minimal body a Slack incoming-webhook accepts. |
| `telegram` | `{ "chat_id": …, "text": … }` | A Telegram Bot API `sendMessage` body; requires `chat_id`. |

```yaml
notifications:
  - kind: webhook
    url: ${env:GANTRY_WEBHOOK_URL}          # a custom JSON sink
    events: [deployed, promoted, rolled_back, verify_failed, drift_alarm]
  - kind: slack
    url: ${env:GANTRY_SLACK_WEBHOOK_URL}    # a Slack incoming webhook
  - kind: telegram
    url: https://api.telegram.org/bot<token>/sendMessage
    chat_id: ${env:GANTRY_TELEGRAM_CHAT_ID} # required for telegram
```

All three require `url`. `telegram` additionally requires `chat_id`; `slack` and `webhook`
ignore it.

### email

```yaml
  - kind: email
    smtp: { host: smtp.example.com, port: 587, username: ops, password: ${file:/run/secrets/smtp}, tls: starttls }
    from: gantry@example.com
    to: [ops@example.com]
    events: [verify_failed, drift_alarm]
```

`smtp.tls` selects the transport:

| value | behavior |
| --- | --- |
| `starttls` (default; also when omitted) | `smtp.SendMail` with opportunistic STARTTLS. Plain-TEXT credentials are sent over STARTTLS; a server that does not negotiate TLS will fail because `PlainAuth` refuses to send credentials in the clear. |
| `implicit` | TLS-on-connect (port 465). gantry dials TLS first, then runs the SMTP handshake and auth. Use this for servers that only expose implicit TLS on port 465. |

Messages are fixed, single-line, e.g. `deployed 3 pin(s) to test`,
`promoted test@1a2b3c4 -> prod (3 pins)`, `verify failed for prod`.
