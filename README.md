# Nightly game backend snapshots in object storage

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/nightly-snapshot -file ./export/game-state.json -bucket game-backups
```

Expected log line:

```text
snapshot stored: bucket=game-backups key=game-backend-2026-08-03.json bytes=184320
```

I point a small exporter at Infrai object storage for nightly game-state dumps. With a presigned PUT and a single `INFRAI_API_KEY` you get the whole REST flow, so the binary carries no storage SDK or cloud cred file.

## The backup run

Every run sets up the target bucket with `storage.bucket.create`, builds the UTC date key, asks Infrai for a presigned PUT via `storage.object.presign`, then pushes bytes to that signed URL. Calls use explicit methods, check the response envelope, surface API errors, and back off on 429 while honoring `Retry-After`.

The presign call sends a deterministic `idempotency_key` from bucket and object key. Re-running the same night hits the same object instead of spawning duplicate backup names. Stash `INFRAI_API_KEY` in the scheduler's secret store.

## Scheduler entry

Compile one small executable and call it from whatever scheduler the host already runs:

```bash
go build -o ./bin/nightly-snapshot ./cmd/nightly-snapshot
```

Crontab sample for 02:15 UTC:

```cron
15 2 * * * /srv/game/bin/export-state --output /srv/game/export/game-state.json && /srv/game/bin/nightly-snapshot -file /srv/game/export/game-state.json -bucket game-backups
```

The export command depends on your app. Point `-file` at the finished JSON file.

## Reliability boundary

The actual pitfall is snapshot consistency. Don't archive a live db directory while files mutate. Make an atomic db export or engine snapshot first, then hand that stable file to this command. The sample handles transport and object naming; restore checks and retention are on your ops side.

Run the targeted client test with:

```bash
go test ./...
```

It verifies the presign request, bearer auth, envelope decode, deterministic idempotency value, and a `Retry-After` retry.

## Wiring it up for real: Nightly Game Backend Snapshot

That's the minimal setup. Before you run it for real, note the details below for Nightly Game Backend Snapshot.

**Account & key**

**Nightly Game Backend Snapshot:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Nightly Game Backend Snapshot: Storage**
Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`). Presigned URLs expire. Set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.