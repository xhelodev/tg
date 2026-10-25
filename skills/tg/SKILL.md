---
name: tg
description: Send Telegram messages, alerts and files from the shell with the `tg` CLI (Telegram Bot API). Use when the user asks to notify, alert or ping them on Telegram, send a file/photo/log to a chat, post to a group, channel or topic, or find out a chat_id.
---

# tg — Telegram from the shell

`tg` is a static CLI over the Telegram Bot API. One bot token, numeric chat IDs,
stable exit codes. Full reference: `tg help` or https://github.com/xhelodev/tg.

## 1. Check it works

```bash
command -v tg || echo "missing"   # install: curl -fsSL https://raw.githubusercontent.com/xhelodev/tg/main/install.sh | bash
tg me -q >/dev/null; echo $?      # 0 = token ok · 3 = no token
```

- **Exit 3 (no token):** ask the user to run `tg login` themselves (it's
  interactive). **Never** ask them to paste the token into the chat, never pass
  `--token`, never `cat` `~/.config/tg/config.json` or print `$TG_BOT_TOKEN`.
- If they already have a token file: `tg me --token-file /path/to/token`.

## 2. Know where to send

- If the profile has `default_chat`, `tg send "text"` goes there.
- Otherwise you need a **numeric chat_id** (`@username` only works for public
  channels and supergroups; it never works for private users). Don't guess it:
  ```bash
  tg whoami --once -q   # then ask the user to message the bot; prints e.g. 589215 or -1001234567890:42
  ```
  In groups the user must write `/whoami` or mention the bot. `whoami` fails
  with exit 4 (409) if the same token is used by a webhook or another running
  bot: say so instead of retrying.
- Topics (forum threads): `--chat -100123:42` or `--chat -100123 --thread 42`.

## 3. Send

```bash
tg send --chat 589215 "build OK ✅"
tg send --chat 589215 --html-escape "$(tail -n 20 build.log)"   # arbitrary text: escape it
some_cmd 2>&1 | tg send --chat 589215 --html-escape --split -   # stdin; --split for >4096 chars
tg send --chat 589215 --document ./report.pdf --caption "informe"
tg send --chat 589215 --photo ./chart.png
tg send --chat 589215 --silent "no notification sound"
```

- **Default parse mode is HTML and text is NOT escaped.** Any `<`, `>` or `&`
  in logs/command output breaks the send (exit 4, "can't parse entities").
  Use `--html-escape` for untrusted text, or `--parse-mode ""` for plain text.
  Only send raw HTML (`<b>`, `<code>`, `<pre>`) you wrote yourself.
- Limits: text 4096 chars (use `--split`), caption 1024, upload 50 MB
  (photos 10 MB).
- `-q` prints only the `message_id`; `--json` prints the API result.
- Flags go **after** the command: `tg send --json …`, not `tg --json send …`.
- Preview without sending: `--dry-run` (the token is redacted).

## 4. Follow up on a message

```bash
id=$(tg send -q --chat 589215 "deploy: en curso…")
tg edit --chat 589215 --message-id "$id" "deploy: listo ✅"
tg react --chat 589215 --message-id "$id" --emoji "👍"
tg delete --chat 589215 --message-id "$id"
```

## 5. Exit codes (branch on these, not on stderr text)

| code | meaning | what to do |
|------|---------|------------|
| 0 | ok | — |
| 2 | bad flags/args | fix the command |
| 3 | no/invalid token | ask the user to run `tg login` |
| 4 | Telegram said no (`ok:false`) | read stderr: chat not found, bot not in chat, parse error… |
| 5 | network/timeout | retry later; 429 rate limits are already retried |
| 130 | Ctrl-C | stop |

## Don'ts

- Don't spam: one message per event; batch or `--split` long output.
- Don't loop `tg whoami` in the background or leave it running unattended.
- Don't send secrets (tokens, `.env`, keys) to Telegram, even to the user's own DM.
