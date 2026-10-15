# Dharma · nghe và luyện tụng

Mobile-first app: SvelteKit 2 / Svelte 5 / Tailwind 4 + Go + PostgreSQL + a Python speech worker. Sample audio and original YouTube JSON3 are in `imports/aaaS89avDVU`.

## Run with PostgreSQL

Start Docker Desktop, then from this directory:

```sh
docker compose up --build
```

Open http://localhost:3000. Database schema and sample are installed on the first API start; subsequent starts preserve database content. For mobile on the same Wi-Fi, open `http://YOUR_MAC_LAN_IP:3000`. Uploaded audio lives in the `audio_uploads` Docker volume; model downloads live in `speech_models`. Optional private MinIO storage is available; see [MinIO setup](docs/minio.md).

## Run without Docker (explicit file preview)

```sh
cd be
go run ./cmd/server -file-mode -auto-ipa=false
```

In a second terminal:

```sh
cd fe
npm ci
npm run dev -- --port 5173
```

Open http://localhost:5173 or the Network URL printed by Vite on your phone. This mode does not use PostgreSQL. For a separately running PostgreSQL, set `DATABASE_URL` and omit `-file-mode`.

## Pages

- `/` opens `/library`. Legacy `/?recording={id}` links redirect to `/recordings/{id}`.
- `/library` lists recordings with search and status filters. **Nhập bài** opens a shared YouTube/MP3 import dialog; successful upload opens the new recording's detail page.
- `/recordings/{id}` contains the player, transcript, IPA, processing status, and segment editor. **← Thư viện** returns to the library. Audio stops on leaving a recording; playback position and preferences remain scoped to that recording. Route changes create a fresh player instance, avoiding audio or selection state leaking between recordings.

## Behavior

- Tap a token to seek and play. Highlight uses audio time, including after seeking or changing speed.
- Choose “Chọn đoạn”, then first and last tokens (either direction). The default is continuous looping with a 0.5-second rest. Selection no longer opens settings automatically.
- Iconify Lucide icons are bundled locally and rendered on the server; no icon API requests. Settings use a separate native dialog with keyboard focus management, rather than expanding the fixed player. Advanced A–B and boundary edits are deferred.
- Pause during a rest pauses its countdown; resuming continues it. Seeking cancels looping.
- Position, speed, text size and selected range persist in local storage on this device.
- Settings include auto-scroll and readable text sizes. Auto-scroll follows the active word line, keeps it in the middle reading band between the sticky header and player, and looks ahead up to 0.4 seconds before line transitions. It pauses during manual scrolling, selection, rest intervals, settings, and transcript editing. Manual scrolling shows **Theo lời đang đọc** to resume. Reduced-motion preferences disable smooth movement. Fixed player accounts for mobile safe areas.
- Original JSON3 is preserved. `import.json` is normalized at load from legacy `words` to canonical `tokens`. Each token has absolute `startMs`/`endMs`, `endEstimated`, and optional `phonetic: {type,text}`. IDs are assigned deterministically.
- Word ending times are display estimates, not acoustic endpoints. Automatic captions and generated IPA need editorial review.

## Import an MP3

Open `/library`, choose **Nhập bài → File MP3**, enter a title, drag an English MP3 into the drop area (or click it to choose a file), and choose **Tải lên & tạo lời**. Files must contain MP3 audio, be at most 200 MB and at most four hours long. The API verifies the actual audio with ffprobe. The page shows a progress bar and percentage while following the durable job through recognition, word alignment, and IPA generation; it can be closed and reopened from the library without stopping processing.

The `transcriber` container runs faster-whisper `small.en` on CPU/int8 for recognition and WhisperX's English wav2vec2 alignment for word times. It processes one job at a time. Only alignment is used from WhisperX, so diarization, pyannote and CUDA dependencies are omitted. Python, FFmpeg, eSpeak, and speech models run inside backend containers; no host installation is needed. Initial model downloads require internet and are cached in `speech_models`. Subsequent jobs reuse those downloads. Processing speed depends on audio length and CPU resources.

Configure the recognition model and CPU threads when starting Compose:

```sh
WHISPER_MODEL=small.en CPU_THREADS=2 docker compose up -d --build
```

In a ready recording, choose **Chỉnh lời**, then **Sửa lời** below a segment. **Nghe đoạn** previews its original audio interval. **Lưu & căn lại** queues an alignment job for the corrected text, generates missing IPA, and atomically replaces only that segment's tokens while preserving its ID and outer time boundaries. Unchanged words at the same token position retain existing pronunciation corrections. Selection/loop markers reset when a revised transcript arrives. Other segments and recordings remain unchanged. Word times without an acoustic alignment score, or whose intervals must be adjusted, carry `endEstimated: true`. The reading view shows one general timing note at the end of the recording; **Chỉnh lời** exposes a collapsible list of estimated token intervals for each affected segment.

Percentages are estimates weighted by processing stage: recognition uses processed audio time (5–70%), alignment uses WhisperX segment callbacks (70–90%), and IPA uses completed words (90–98%). Editing skips recognition and allocates 5–90% to alignment. Saving displays 99%; only the committed ready state displays 100%. Progress is stored in PostgreSQL and remains visible after reopening the page, including in the library. Model preparation has a separate message while its progress cannot be measured. Retrying or recovering an interrupted worker starts a new attempt at 0%.

Failed jobs show an error and a retry button. Existing playable text survives failed corrections. Worker restart recovers interrupted recognition/alignment jobs; API restart recovers interrupted IPA jobs. A PostgreSQL partial unique index prevents concurrent active tasks on one recording, and transcript revisions reject stale edits. Jobs, transcripts and audio survive container restarts. The sample import and original JSON3 are preserved. Sample playback uses a 192 kbps CBR MP3 to avoid VBR fast-seek offsets; the original audio is retained as `imports/aaaS89avDVU/audio.original-vbr.mp3`. Its playback URL includes a version query to refresh browser caches after this replacement.

API endpoints (same-origin through the SvelteKit proxy):

- `POST /api/recordings`: multipart `title` and `audio`; returns `recordingId` and `jobId` with HTTP 202.
- `GET /api/sutras`: library with recording IDs and processing status.
- `GET /api/recordings/{id}/job`: latest job's stage and error.
- `GET /api/recordings/{id}/transcript`: player data; HTTP 409 until first transcript is ready.
- `GET /api/recordings/{id}/audio`: audio with Range support.
- `PATCH /api/recordings/{id}/segments/{segmentId}`: JSON `{ "text": "corrected English" }`; returns HTTP 202.
- `POST /api/recordings/{id}/retry`: requeues the latest failed job.

File-only preview supports the sample player; upload/edit processing requires PostgreSQL and the Docker worker.

## Automatic subtitle IPA

The API container includes eSpeak NG. On a fresh database, it generates the sample’s British English IPA and stores it in PostgreSQL before serving requests. Existing named phonetics are reused, including manual corrections. Legacy recordings retain their unlabelled IPA until a named version is requested. Restarts reuse stored IPA. Token IDs, text, timing, and multiword token boundaries remain unchanged. Repeated identical token text is generated once per run.

```sh
docker compose up -d --build
```

No phonetics tool needs to be installed on the host. File preview with `-auto-ipa=false` above deliberately skips generation; use Docker to preview IPA.

The default pronunciation for new imports is British English. Use **Cài đặt nghe → Phiên âm IPA** to request American English or change a recording’s default. Changing the displayed voice does not overwrite stored IPA. IPA is a standard text-based pronunciation, not an acoustic transcription of the recording; homographs, proper names and Buddhist terminology need review. Missing eSpeak or generation errors stop startup with the affected token in the log, rather than silently saving partial results.

`imports/aaaS89avDVU/phonetic-overrides.json` contains a JSON map from words or phrases to reviewed IPA strings (without `/` delimiters). Matching ignores capitalization and outer punctuation. Add reviewed terms there before generation and restart the API with `docker compose restart api`, then use **Tạo lại IPA** for an existing automatic version. Edit reviewed pronunciations in `token_ipa.phonetic` with `source: "manual"` to preserve them when regenerating. This avoids overwriting editorial corrections. No unverified terminology overrides are supplied.

In **Cài đặt nghe**, **Hiển thị phiên âm IPA** controls the line below each token. It is on by default and saved on the current device. The original JSON3 and import file remain unchanged.

## Verification

```sh
cd fe
npm run check
npm test
npm run build
```

```sh
cd be
go test ./...
go vet ./...
```

Alignment normalization tests:

```sh
python3 -m unittest discover -s transcriber
```

PostgreSQL integration tests create and remove their own temporary schema. Set `TEST_DATABASE_URL` to a reachable test database and run `go test ./...` from `be`; these tests cover recording isolation, atomic segment edits, preserved IPA, concurrent jobs, and stale revisions.

## Current scope

One sample recording and automatic first-run import. YouTube import, English MP3 upload, durable background processing, multiple recordings, and segment text correction are supported. No authentication, offline audio downloads, or cross-device sync yet. Private MinIO storage is optional; see [MinIO setup](docs/minio.md). HTTP audio supports Range requests through the same-origin SvelteKit proxy.


### IPA accents per recording

New MP3 imports generate **only British English (`en-gb`) IPA**. American English (`en-us`) is generated only on request. In **Cài đặt nghe → Phiên âm IPA**, select **Anh–Anh** or **Anh–Mỹ**, then create the missing version or regenerate the selected version. Switch between completed versions immediately and use **Đặt mặc định cho bài** to save the default for that recording. Generating another accent does not change the recording default automatically (except the first named version for a legacy recording).

IPA tasks run in the API's durable PostgreSQL queue; they do not transcribe or align audio again. Results are saved atomically without changing transcript revision, word timings, playback or selected loop range. Regeneration keeps manual/unannotated corrections within their accent and replaces automatic IPA only. Editing transcript text generates its default accent only; unchanged words retain other-accent IPA, while incomplete versions must be generated again before selection.

Legacy IPA remains available without an inferred accent label. No bulk IPA generation runs during migration. A legacy recording gets an explicit default when its first named version completes. File-mode preview retains its existing IPA; per-recording generation and defaults require PostgreSQL mode.

API endpoints:
- `POST /api/recordings/{id}/ipa` with `{"voice":"en-gb"}` or `{"voice":"en-us"}` queues generation/regeneration.
- `PATCH /api/recordings/{id}/ipa/default` with the same body saves a completed accent as the recording default.
- Transcript responses include `defaultIpaVoice`, `ipaVoices`, `ipaEditable`, and each token's `phonetics` by voice. The existing `phonetic` field contains the default version (or unlabelled legacy IPA).


### Readable IPA display

**Cài đặt nghe → Cách hiển thị IPA** defaults to **IPA dễ đọc**; choose **IPA gốc** to see the exact stored output. This device preference is saved per recording and is independent of the recording’s British/American default. Existing IPA is rendered immediately without regenerating it.

The API adds a response-only `readable` field. Word-level alignment against the locally bundled [CMU Pronouncing Dictionary](https://github.com/cmusphinx/cmudict) resolves `ᵻ` to `ɪ`/`ə` and `ɾ` to the word’s `t`/`d`. The dictionary supplies only these ambiguous sounds, preserving the selected accent’s other vowels. `ɐ` uses the simpler unstressed `ə` notation, while `ɹ`/`ɛ` display as `r`/`e`. Meditative follows the [Oxford learner forms](https://www.oxfordlearnersdictionaries.com/definition/english/meditative) for each accent. This display convention simplifies detail and does not claim to reproduce every pronunciation variant.

For an out-of-dictionary or poorly aligned word, reduced `ᵻ` falls back to `ɪ`. Automatic eSpeak `ɾ` restores its `t#` as `t`; an unresolved tap in a manual/legacy pronunciation remains unchanged instead of guessing. Original IPA, transcript text, timestamps, loop selections and stored manual corrections are preserved. Import runs only its existing single-accent generation; readable conversion adds no external requests or synthesis runs.

CMUdict attribution, pinned version and full redistribution terms are in `be/internal/phonetics/data/README.md` and `be/internal/phonetics/data/CMUDICT-LICENSE`. The API Docker image includes the license at `/app/licenses/CMUDICT-LICENSE`.


Readable IPA stress follows the [Oxford pronunciation guide](https://www.oxfordlearnersdictionaries.com/about/english/pronunciation_english): stress marks precede syllable onsets, single-syllable words omit stress, and secondary stress after the primary is omitted. Examples: `rˈaɪt → raɪt`, `əbˈaʊt → əˈbaʊt`, `ɪmpˈɔːtənt → ɪmˈpɔːtənt`. A local syllable/onset normalizer retains the source's stressed vowel and accent; syllabic consonants and centring diphthongs are checked against the bundled lexicon. This is Oxford-style stress notation, not a complete Oxford pronunciation database; unusual names, compounds and pronunciation variants may still need editorial review. IPA gốc remains exact stored output. No regeneration or import-time synthesis is added.

## PWA and browser rendering

The app uses `ssr = false` in `fe/src/routes/+layout.ts`. The `/` redirect (including `?recording=…`) runs in the universal `+page.ts`. `adapter-static` exports the SPA with an `index.html` fallback. Caddy serves the build on port 3000 and proxies same-origin `/api/*` directly to the Go API; no Node runtime is used in production. For local Vite development, `/api/*` is proxied to `API_ORIGIN` (default `http://localhost:8080`). Absolute asset paths let one cached CSR shell open `/library`, `/recordings/{id}` and `/` directly while offline.

Production builds include `/manifest.webmanifest`, the generated PQ Dharma bodhi-leaf/play logo, 192/512 PNG launcher icons (including maskable), a 180px Apple icon and 32px favicon. Logo master: `fe/static/branding/pq-dharma-logo.png`. The exact built-in image_gen prompt and export provenance are in `docs/branding/logo-prompt.md`.

The service worker is registered manually by `PwaControls.svelte` in production secure contexts. It precaches the UI and its dependencies, then keeps successful library/transcript JSON responses (up to 60 transcripts) available offline. Processing/job state, MP3 media/Range requests, uploads and mutations always use the network. Offline UI explains the limits and disables operations that require a connection. On reconnect, live status resumes. The first worker activation refreshes page data to cache a recording opened before the worker gained control.

This PWA phase supports installation and reading previously opened words/IPA offline. **Full audio downloads and offline audio seeking are not implemented.** Initial access, import, transcription, IPA generation and audio playback require a connection. Browser storage can be evicted; these caches are not a backup of imported recordings.

Updates install in the background and wait. **Cập nhật & tải lại** explicitly activates the waiting worker; there is no automatic reload during playback. Cache cleanup affects only PQ Dharma's own versioned shell caches, retaining the preceding bundle for other open tabs and preserving saved transcript data.

Use `docker compose up -d --build web` and open `http://localhost:3000` to test a production build locally. Service workers are intentionally disabled under `npm run dev`. For phones or a deployed domain, serve the app over HTTPS through your reverse proxy . HTTP on a LAN IP is not a secure context for service workers. Chrome/Edge expose installation in the app banner or browser menu; on iPhone/iPad use Safari → Share → Add to Home Screen.

Validation: `npm --prefix fe run check`, `npm --prefix fe test`, `npm --prefix fe run build`. Worker tests execute the real handlers with CacheStorage/network doubles to check offline deep links, fresh/saved JSON, Range/mutation bypass, deleted-record handling, and explicit update activation.

## Import from YouTube

In `/library`, choose **Nhập bài**, then **Link YouTube** or **File MP3** in the same dialog. Pasting a video URL automatically fills the title; you can edit it before import. Late metadata responses do not overwrite edits. If metadata is unavailable, enter a title manually.

`POST /api/youtube/preview` accepts `{ "url": "…" }` and returns a title and canonical URL. `POST /api/recordings/youtube` accepts `{ "url": "…", "title": "…" }` and queues the existing durable transcription pipeline. Only single YouTube watch, short-link, shorts and embed URLs are accepted; playlist parameters are ignored.

The worker uses yt-dlp with Deno/EJS, downloads one completed public video, and converts audio to 192 kbps CBR MP3. The final MP3 must be at most 200 MB and four hours. English author captions are preferred over automatic captions; missing/unusable captions fall back to Whisper recognition. Automatic rolling captions are deduplicated, then all text is aligned against the actual MP3 and enriched with British IPA. This imports English speech, not translations of other-language speech.

Original captions (`.captions.json3`) and source provenance (`.source.json`) live beside the audio in the upload volume. The player transcript JSON is exposed through the existing transcript endpoint. Completed MP3 downloads are reused on retry and restart. YouTube access restrictions and rate limits can still prevent a download; the job reports failure with the existing retry control. MP3 upload remains available.

Apply with `docker compose up -d --build api transcriber web`.

## Private MinIO storage

See [setup, migration, object layout, and operational checklist](docs/minio.md). With S3 enabled, audio and source assets live in MinIO; PostgreSQL remains the source for editable transcripts/IPA. The same-origin audio API preserves Range playback. Leave `S3_ENDPOINT` empty for local-only storage.
