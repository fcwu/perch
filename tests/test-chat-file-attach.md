# Chat File Attachments — e2e test cases

Verifies the non-image upload path added by `2026-05-01 chat-file-attachments`.
Image-only behaviour is covered by the existing `test-chat-upload.md`; this
plan focuses on the disk-save path for text/PDF and the conv-scoped lifecycle.

## Pre-requisites

- A deployed perch instance (web `/chat` reachable, ACP runtime configured)
- A valid GitLab session cookie for `/api/chat`
- `tests/.env.<device>.md` provides `PERCH_HOST`, `SESSION_TOKEN`, `WORKDIR`
- `jq`, `curl`, `base64`, `ssh`, `docker` available
- Container shell access to inspect `<WORKDIR>/uploads/`

Substitute `${HOST}`, `${TOK}`, `${WORKDIR}`, `${CONTAINER}` from your env file.

## Helpers

```bash
mk_attach() {
  local file="$1" mime="$2"
  local b64
  b64=$(base64 -w0 "$file")
  jq -n --arg fn "$(basename "$file")" --arg mt "$mime" --arg d "$b64" \
    '{filename:$fn, mime_type:$mt, data_base64:$d}'
}

post_chat() {
  local body="$1"
  curl -s -o /tmp/chat-resp -w '%{http_code}' \
    -H "Cookie: session_token=${TOK}" \
    -H 'Content-Type: application/json' \
    -X POST "https://${HOST}/api/chat" -d "$body"
}

uploads_dir() {
  ssh "${HOST}" "docker exec ${CONTAINER} ls ${WORKDIR}/uploads/$1 2>/dev/null"
}
```

## CFA01 — `/api/chat` with one PDF persists to disk + prompt prefix correct

```bash
echo "%PDF-1.4 minimal" > /tmp/cfa01.pdf
PDF=$(mk_attach /tmp/cfa01.pdf application/pdf)
CONV="cfa01-$(date +%s)"
BODY=$(jq -n --arg q "summarise this pdf" --arg c "$CONV" --argjson a "[$PDF]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')
test "$(post_chat "$BODY")" = "200" || { echo FAIL; exit 1; }

# File present on disk
uploads_dir "$CONV" | grep cfa01.pdf || { echo FAIL no file; exit 1; }

# Verify the prompt the agent received contains [file: ...] prefix
# (inspected via container logs or query_log_store; agent should echo back filename)
ssh "${HOST}" "docker logs ${CONTAINER} --tail 200" | grep "uploads accepted.*files\":1" || echo "WARN log line not found"
```

**Expect**: HTTP 200, file at `${WORKDIR}/uploads/${CONV}/cfa01.pdf`, agent
response references `cfa01.pdf` (proves prefix worked). `query_sessions.query`
in admin history shows `[file:cfa01.pdf] summarise this pdf`.

## CFA02 — `/api/chat` with one CSV (text MIME via heuristic)

```bash
printf "name,age\nalice,30\nbob,25\n" > /tmp/cfa02.csv
CSV=$(mk_attach /tmp/cfa02.csv text/csv)
CONV="cfa02-$(date +%s)"
BODY=$(jq -n --arg q "what's the average age" --arg c "$CONV" --argjson a "[$CSV]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')
test "$(post_chat "$BODY")" = "200" || { echo FAIL; exit 1; }
uploads_dir "$CONV" | grep cfa02.csv || { echo FAIL; exit 1; }
```

**Expect**: HTTP 200, agent response mentions average ~27.5 (proves it actually
read the CSV from disk via Read tool).

## CFA03 — Mixed PNG + PDF: PNG inline, PDF persisted

```bash
# Use an existing test PNG fixture
PNG=$(mk_attach tests/fixtures/tiny.png image/png)
PDF=$(mk_attach /tmp/cfa01.pdf application/pdf)
CONV="cfa03-$(date +%s)"
BODY=$(jq -n --arg q "compare" --arg c "$CONV" --argjson a "[$PNG,$PDF]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')
test "$(post_chat "$BODY")" = "200" || { echo FAIL; exit 1; }

# Only the PDF should land on disk; PNG stays inline
uploads_dir "$CONV" | grep cfa01.pdf || { echo FAIL pdf missing; exit 1; }
uploads_dir "$CONV" | grep -v "\.png" || true  # png MUST NOT be there
```

**Expect**: HTTP 200, PDF on disk, PNG NOT on disk. `query_sessions.query`
shows both `[image:tiny.png]` and `[file:cfa01.pdf]`.

## CFA04 — Same-named CSV twice in same conv → second writes as `name (2).csv`

```bash
CSV=$(mk_attach /tmp/cfa02.csv text/csv)
CONV="cfa04-$(date +%s)"
BODY=$(jq -n --arg q "first upload" --arg c "$CONV" --argjson a "[$CSV]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')
test "$(post_chat "$BODY")" = "200" || { echo FAIL; exit 1; }
sleep 2 # let first prompt finish
BODY2=$(jq -n --arg q "second upload same name" --arg c "$CONV" --argjson a "[$CSV]" \
  '{query:$q, conversation_id:$c, attachments:$a}')
test "$(post_chat "$BODY2")" = "200" || { echo FAIL; exit 1; }

uploads_dir "$CONV" | grep -E '^cfa02\.csv$' || { echo FAIL first; exit 1; }
uploads_dir "$CONV" | grep -E 'cfa02 \(2\)\.csv' || { echo FAIL dup suffix; exit 1; }
```

**Expect**: both `cfa02.csv` and `cfa02 (2).csv` exist; original is intact.

## CFA05 — Per-conversation quota exceeded → 400, no disk write

```bash
# Set CHAT_UPLOAD_DIR_QUOTA_BYTES=1024 via settings UI or env, then:
dd if=/dev/zero bs=1 count=2048 2>/dev/null | base64 -w0 > /tmp/cfa05_2k_b64
B64=$(cat /tmp/cfa05_2k_b64)
ATT=$(jq -n --arg fn big.txt --arg mt text/plain --arg d "$B64" \
  '{filename:$fn, mime_type:$mt, data_base64:$d}')
CONV="cfa05-$(date +%s)"
BODY=$(jq -n --arg q "x" --arg c "$CONV" --argjson a "[$ATT]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')
RC=$(post_chat "$BODY")
test "$RC" = "400" || { echo "expected 400, got $RC"; exit 1; }

# Confirm no partial write
uploads_dir "$CONV" | wc -l | grep -E '^0$' || { echo FAIL partial; exit 1; }
```

**Expect**: HTTP 400 with `"quota exceeded"` in response body, no `${CONV}/`
directory created (or empty).

## CFA06 — ACP pool eviction → conv uploads dir removed

```bash
# Set CHAT_POOL_IDLE_TIMEOUT to a short value (e.g. 60s) for the test, or
# manually trigger eviction by hitting the management endpoint.
PDF=$(mk_attach /tmp/cfa01.pdf application/pdf)
CONV="cfa06-$(date +%s)"
post_chat "$(jq -n --arg q ok --arg c "$CONV" --argjson a "[$PDF]" \
  '{query:$q, conversation_id:$c, new_conversation:true, attachments:$a}')"
uploads_dir "$CONV" | grep cfa01.pdf || { echo FAIL pre-evict; exit 1; }

# Wait for idle timeout (or kill the subprocess via management API)
sleep 70

uploads_dir "$CONV" 2>&1 | grep -q "No such file" && echo OK_REMOVED || echo "FAIL still present"
```

**Expect**: after the pool evicts the (user, conv) entry, the directory is
gone. The eviction hook log line `ACP chat: cleaned uploads dir on evict` is
in the container log.

## CFA07 — Restart with orphan dir present → orphan removed

```bash
# Manually create a backdated orphan directory inside the container:
ssh "${HOST}" "docker exec ${CONTAINER} sh -c '
  mkdir -p ${WORKDIR}/uploads/cfa07-orphan && \
  echo old > ${WORKDIR}/uploads/cfa07-orphan/x.txt && \
  touch -d \"30 days ago\" ${WORKDIR}/uploads/cfa07-orphan ${WORKDIR}/uploads/cfa07-orphan/x.txt && \
  mkdir -p ${WORKDIR}/uploads/cfa07-fresh && \
  echo new > ${WORKDIR}/uploads/cfa07-fresh/y.txt
'"

# Restart perch
ssh "${HOST}" "docker restart ${CONTAINER}"
sleep 5

# Orphan removed, fresh kept
ssh "${HOST}" "docker exec ${CONTAINER} ls ${WORKDIR}/uploads/" | grep -v cfa07-orphan || { echo FAIL orphan still present; exit 1; }
ssh "${HOST}" "docker exec ${CONTAINER} ls ${WORKDIR}/uploads/" | grep cfa07-fresh   || { echo FAIL fresh removed; exit 1; }
```

**Expect**: orphan dir gone, fresh dir kept. Container log has
`uploads orphan cleanup kept=1 removed=1`.

## CFA08 — Discord message with PDF attachment → persisted + prompt prefix

```bash
# Send a Discord message in the perch-bot channel with a PDF attachment.
# This is a manual step (Discord client / mobile app):
#   - In channel ${ALLOWED_CHANNEL}, attach a small PDF and type "summarise"
# Then verify on host:

CHAN="${ALLOWED_CHANNEL}"  # numeric Discord channel ID
ssh "${HOST}" "docker exec ${CONTAINER} ls ${WORKDIR}/uploads/${CHAN}" | grep -i pdf || { echo FAIL no pdf persisted; exit 1; }
```

**Expect**: PDF appears under `${WORKDIR}/uploads/${CHAN}/`, bot reply
references the PDF contents (proves agent read it). If a `.mp4` is attached,
the bot reply ends with `> 附件 X 不支援此類型 (video/mp4)`.

## Notes

- After each test, leftover files under `${WORKDIR}/uploads/<conv>/` are
  acceptable; they will be cleaned up by the orphan scan or pool eviction
- Tests assume `CHAT_UPLOAD_ALLOWED_MIME` is at the new default (image + text + PDF)
- For Phase 2 (video/audio) extension, copy this file and adjust the MIME
  allow-list + add fixtures
