# Deploy configuration

Infrastructure configuration that a deployment applies, kept as code so it is
reviewed and versioned with the backend that depends on it. Nothing here runs
in local development; `docker-compose.yaml` is the local equivalent.

Values that differ per environment are written as `${NAME}` placeholders and
replaced from the deployment's environment when the file is applied. No file
here holds a secret.

## Bucket CORS (`gcs-cors.json`)

The web frontend's GameHost downloads a started Session's game frontend and its
assets from the signed Cloud Storage URLs in `session_started` with `fetch`: it
must read the bytes to verify their SHA-256 before running them, so the bucket
must answer CORS for the platform's web origin, or browsers refuse every
download and no game loads.

The rule allows `GET` and `HEAD` from `${PLATFORM_WEB_ORIGIN}` (the origin the
web frontend is served from, such as `https://play.example.com`; the same value
as `CORS_ALLOWED_ORIGINS` and `WEBSOCKET_ALLOWED_ORIGINS` of the API), lets the
page read `Content-Type`, and lets the browser reuse a preflight for an hour.
Signed URLs carry no credentials, so nothing else is needed. Keep it to the
platform's origins, never `*`. To allow several origins, list each one in
`origin`.

Apply it to the bucket named by `GCS_BUCKET`, with the platform's origin:

```sh
PLATFORM_WEB_ORIGIN=https://play.example.com envsubst < deploy/gcs-cors.json > /tmp/gcs-cors.json
gcloud storage buckets update gs://<bucket> --cors-file=/tmp/gcs-cors.json
gcloud storage buckets describe gs://<bucket> --format="default(cors_config)"
```

Check it before the web frontend uses the bucket, with a signed URL from a
`session_started`:

```sh
curl -sI -H "Origin: https://play.example.com" "<signed url>" | grep -i access-control-allow-origin
```

In local development the fake-gcs emulator of `docker-compose.yaml` already
allows any origin, so nothing needs applying there.
