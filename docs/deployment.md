# Server deployment

The server provides `posgres` (PostgreSQL 17) and `nginx_network`.
Compose does not manage their lifecycle. All four application containers join
this network. The web container serves the SPA and proxies `/api/*` to the API.

## Database and environment

Keep `.env` private and outside Git. Set `DATABASE_URL` for a dedicated `pqdharma`
role and database, using host `posgres` and port `5432`. URL-encode special
characters in the password. Preserve existing S3 settings; an empty
`S3_ENDPOINT` selects local storage.

Before starting the app, connect as the shared PostgreSQL administrator:

```sh
docker exec -it posgres sh -c 'psql -U "${POSTGRES_USER:-postgres}" -d postgres'
```

For a new installation, execute the following in psql. At the password prompt,
use the dedicated application's password from `.env` (decoded if URL-encoded):

```sql
CREATE ROLE pqdharma LOGIN;
\password pqdharma
CREATE DATABASE pqdharma OWNER pqdharma;
\quit
```

The API runs schema migrations and seeds the sample at startup, so its role
must own the app database and have permission to create/alter its tables.
Use a dedicated database rather than another application's database.

## Start and route traffic

```sh
docker network inspect nginx_network --format '{{.Name}}'
docker compose config --quiet
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 api transcriber tts web
```

In Nginx Proxy Manager, add a Proxy Host for the chosen domain:

- Forward scheme: `http`
- Forward hostname: `pq-dharma-web`
- Forward port: `3000`
- Enable a valid SSL certificate and Force SSL for PWA support.
- Add `client_max_body_size 210m;` in Advanced to allow MP3 uploads up to 200 MB.
- Configure an Access List if this is a private deployment; the app has no login.

Do not add a separate public API route. Web already proxies `/api/*` and audio
Range requests. No host port binding is required, avoiding the server's occupied
port 3000.

The API healthcheck verifies `/api/health`, including database connectivity.
Web and worker start after the API becomes healthy. Initial IPA preparation may
take time. Services restart unless manually stopped; a failed startup dependency
can be retried with `docker compose up -d` once PostgreSQL is available.

The worker defaults to two CPUs, two speech threads and a 4 GiB memory limit.
Adjust `TRANSCRIBER_CPUS`, `CPU_THREADS`, `TRANSCRIBER_MEMORY`, and `WHISPER_MODEL`
in `.env` if larger models or recordings need more resources. Initial builds and
model downloads require internet and additional disk space.

## Persistent data

Keep `audio_uploads`, `speech_models`, and `tts_models` volumes and the sample bind mount under
`imports/aaaS89avDVU`. If `import.json` is absent, PostgreSQL mode starts without seeding the sample.
Existing recordings remain available; file preview still requires a valid sample.
Sample audio playback and worker alignment require the original sample audio.
Back up the app database and media (local volume or MinIO). Model cache can be
downloaded again. Avoid `docker compose down -v` when retaining uploaded media.
Stopping this Compose project does not stop or delete the shared PostgreSQL.

## Local PDF speech service

`pq-dharma-tts` is internal on port 8000; Nginx Proxy Manager still forwards
only to `pq-dharma-web:3000`. No new public route or PostgreSQL container is
needed. The transcriber waits for the TTS process healthcheck before starting.
The first PDF job downloads Kokoro's model/voices to `tts_models`; allow outbound
HTTPS for that initial download. Offline operation also requires the existing
WhisperX alignment model to have been cached in `speech_models`.

Set `TTS_CPUS=2`, `TTS_CPU_THREADS=2`, and `TTS_MEMORY=2g` initially. The alignment
worker and TTS service may hold models in memory at the same time; measure actual
usage on the Ubuntu host and adjust these limits if necessary. View synthesis
errors with `docker compose logs --tail=100 tts transcriber`. A failed PDF job
can be resumed using **Thử lại** in its recording page.

## Local Docker database

`compose.local.yaml` adds a PostgreSQL 17 container and binds web to
`http://localhost:3000` on the local machine. The main Compose file continues
to use an external database on Ubuntu. Create `nginx_network` locally once if
it is absent. Set `LOCAL_POSTGRES_PASSWORD` in the private `.env` to a random
administrator password. Start the local database:

```sh
docker compose -f compose.yaml -f compose.local.yaml up -d postgres-local
docker exec -it pq-dharma-local-postgres psql -U postgres -d postgres
```

On a new local volume, create the app role/database once (skip creation if they
already exist), using a separate app password at the interactive prompt:

```sql
CREATE ROLE pqdharma_local LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE;
\password pqdharma_local
CREATE DATABASE pqdharma_local OWNER pqdharma_local;
\quit
```

Set `DATABASE_URL` in `.env` to
`postgres://pqdharma_local:APP_PASSWORD@pq-dharma-local-postgres:5432/pqdharma_local?sslmode=disable`
with the app password URL-encoded, then run:

```sh
docker compose -f compose.yaml -f compose.local.yaml up -d --build
```

The API installs the schema and imports the sample when the sample files are
present. The database persists in `local_postgres_data`. Keep using both Compose
files for local starts so the web port and database dependency are included.
Credentials stay in the Git-ignored `.env`; the app itself has no login accounts.

## Recording sharing and link previews

Set `PUBLIC_ORIGIN=https://pqdharma.hatbuinho.me` (replace with the deployed
origin, with no path). Caddy forwards `/recordings/:id` HTML requests to Go,
which adds escaped title/description, canonical URL, Open Graph and Twitter
metadata to the current static `/index.html` shell fetched internally from
`http://pq-dharma-web:3000`. `WEB_SHELL_ORIGIN` can override this internal origin
when running the API outside Compose. Assets and APIs retain their existing
routes. No user-agent sniffing is used. Missing recordings return 404.

The player Share button uses native sharing when supported, otherwise copies
the canonical route on the current origin. The preview image uses the recording
thumbnail when available, with the app logo as fallback. Preview bots must be
able to fetch both the page and image without authentication. Existing social
previews may remain cached by the messaging platform after a metadata change.
Local Compose explicitly uses `http://localhost:3000` as its origin.

## Accounts and recording privacy

Before the first authenticated startup, set `ADMIN_EMAIL` and a strong
`ADMIN_PASSWORD` (12–72 characters) in the private `.env`. Startup creates this
admin if the email is new and assigns unowned existing recordings to it. Existing
passwords are never overwritten. Existing recordings become private by default.
The local bootstrap account is configured in the local `.env`; inspect that
file for its password rather than committing or printing it.

Admins can create users from the topbar profile menu. Each user owns a separate
library. Owner card management changes title, visibility (private/shared/public),
recipient email addresses, or soft-deletes a recording. Trash restores it. Shared
recipients must already have accounts and have listen-only access. Only public
recordings appear in Explore and expose public OG previews. App sessions use
hashed random tokens and HttpOnly SameSite cookies, with Secure enabled for an
HTTPS PUBLIC_ORIGIN. Configure PUBLIC_ORIGIN accurately for CSRF origin checks.

**Remove anonymous/public MinIO bucket access before deploying private content.**
The app authorizes media via its API; a public bucket bypasses those checks.
No MinIO bucket permissions are changed automatically. Existing downloaded files
cannot be recalled. This version stops caching library/transcript data offline
and removes old account data caches when the updated service worker activates.
Users should close old tabs and apply the new PWA build during this migration.
No self registration, password reset or email delivery is included in this release.

### Account administration

Administrators can create regular users or additional administrators from `/users`.
New accounts default to the regular user role. Deactivating an account revokes its
sessions and blocks login; existing recordings, sharing grants, and roles remain
stored. Reactivating restores login with the existing password. Users who are
inactive are omitted from the sharing search. Administrators cannot deactivate
their own account; concurrent changes also preserve an active administrator.
Migration `008_user_status.sql` runs automatically when the API starts.

Signed-in users can change their own password from the account menu. This requires
the current password, a different new password (at least 12 characters, up to 72
UTF-8 bytes), and confirmation in the form. Successful changes revoke all sessions
and prompt the user to sign in again.

The library Share button opens recording access settings. Owners can add/remove
existing accounts as viewers and select restricted or public access. Restricted
access with no viewers maps to private; with viewers it maps to shared. Public
recordings are also discoverable in Explore. Named grants are retained while
public so they remain available when returning to restricted access. Copying a
link does not grant access. Permission changes use a dedicated access endpoint;
renaming a recording preserves its current permissions.
