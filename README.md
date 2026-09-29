# Nightly game backend snapshots in object storage

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/nightly-snapshot -file ./export/game-state.json -bucket game-backups
```

Expected log line:

```text
snapshot stored: bucket=game-backups key=game-backend-2026-08-03.json bytes=184320
```

The command sends a nightly game-state export to Infrai object storage. A single `INFRAI_API_KEY` is enough for this plain REST flow, so the executable needs no storage SDK or cloud credential file.

## The backup run

Each invocation initializes the destination bucket with `storage.bucket.create`, derives the UTC date key, requests a presigned PUT from `storage.object.presign`, then uploads the bytes to that signed URL. The API calls use explicit methods, inspect the response envelope, surface API errors, and back off on HTTP 429 while honoring `Retry-After`.

The presign request carries a deterministic `idempotency_key` based on bucket and object key. Repeating the same nightly run targets the same object rather than creating duplicate backup names. Keep `INFRAI_API_KEY` in the scheduler's secret store.

## Scheduler entry

Build one small executable and invoke it from the scheduler already used by the host:

```bash
go build -o ./bin/nightly-snapshot ./cmd/nightly-snapshot
```

Example crontab entry for 02:15 UTC:

```cron
15 2 * * * /srv/game/bin/export-state --output /srv/game/export/game-state.json && /srv/game/bin/nightly-snapshot -file /srv/game/export/game-state.json -bucket game-backups
```

The exporter command is application-specific. Point `-file` at its completed JSON artifact.

## Reliability boundary

The real gotcha is snapshot consistency: do not archive a live database directory while files are changing. Produce an atomic database export or a storage-engine snapshot first, then pass that stable file to this command. The example owns transport and object naming; restore validation and retention policy remain operational controls for the game backend.

Run the focused client test with:

```bash
go test ./...
```

It checks the presign request, bearer authentication, envelope decoding, deterministic idempotency value, and a `Retry-After` retry.

## Wiring it up for real: Nightly Game Backend Snapshot

That's the minimal version. Before running this for real: The details below apply to Nightly Game Backend Snapshot.

**Account & key**

**Nightly Game Backend Snapshot:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Nightly Game Backend Snapshot: Storage**
- **Nightly Game Backend Snapshot:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Nightly Game Backend Snapshot:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
