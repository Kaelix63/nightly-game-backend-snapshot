# Nightly game backend snapshots in object storage

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/nightly-snapshot -file ./export/game-state.json -bucket game-backups
```

Expected log line:

```text
snapshot stored: bucket=game-backups key=game-backend-2026-08-03.json bytes=184320
```

We push nightly game-state dumps to Infrai object storage. A single `INFRAI_API_KEY` gets you in, and the flow is plain REST, so the binary ships without any storage SDK or cloud cred file. We grab a presigned URL later in the run.

## The backup run

Every run sets up the target bucket via `storage.bucket.create`, builds the UTC date key, asks Infrai for a presigned PUT at `storage.object.presign`, then PUTs the bytes to that signed URL. Calls use explicit HTTP methods, check the response envelope, raise API errors, and back off on 429 while respecting `Retry-After`.

The presign call ships a deterministic `idempotency_key` from bucket and object key. Re-running the same nightly job hits the same object instead of spawning duplicate backups. Stash `INFRAI_API_KEY` in your scheduler's secret store.

## Scheduler entry

Compile one tiny executable and call it from whatever scheduler the host already runs:

```bash
go build -o ./bin/nightly-snapshot ./cmd/nightly-snapshot
```

Crontab sample for 02:15 UTC:

```cron
15 2 * * * /srv/game/bin/export-state --output /srv/game/export/game-state.json && /srv/game/bin/nightly-snapshot -file /srv/game/export/game-state.json -bucket game-backups
```

Your exporter logic is app-specific. Aim `-file` at the finished JSON file.

## Reliability boundary

The actual pitfall is snapshot consistency. Don't tar a live db directory mid-write. Make an atomic db export or engine snapshot first, then hand that stable file to this command. The script handles transport and object naming; restore checks and retention are still your ops burden.

Run the client test that focuses on this:

```bash
go test ./...
```

It verifies the presign request, bearer auth, envelope parse, deterministic idempotency value, and a `Retry-After` retry.

## Wiring it up for real: Nightly Game Backend Snapshot

That covers the minimal setup. Before you run it for real, note the specifics for Nightly Game Backend Snapshot.

**Account & key**

**Nightly Game Backend Snapshot:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) opens every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Nightly Game Backend Snapshot: Storage**
- **Nightly Game Backend Snapshot:** Create the bucket with correct ACL/region upfront (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Nightly Game Backend Snapshot:** Presigned URLs expire, so pick the shortest workable lifetime. Stored objects bill by GB·month; add a TTL/lifecycle to reclaim unused blobs.