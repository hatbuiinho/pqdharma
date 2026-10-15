# Server deployment

The server provides `posgres` (PostgreSQL 17) and `nginx_network`.
Compose does not manage their lifecycle. All three application containers join
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
docker compose logs --tail=100 api transcriber web
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

Keep `audio_uploads` and `speech_models` volumes and the sample bind mount under
`imports/aaaS89avDVU`. If `import.json` is absent, PostgreSQL mode starts without seeding the sample.
Existing recordings remain available; file preview still requires a valid sample.
Sample audio playback and worker alignment require the original sample audio.
Back up the app database and media (local volume or MinIO). Model cache can be
downloaded again. Avoid `docker compose down -v` when retaining uploaded media.
Stopping this Compose project does not stop or delete the shared PostgreSQL.
