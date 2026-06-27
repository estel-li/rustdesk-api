# Client Builder Base Directory

This directory holds the *base* Windows portable EXEs that the lightweight
Client Builder (CE-M1-9) uses as a source for "copy + rename" downloads.

The Client Builder never compiles or signs binaries. It simply copies a base
EXE and renames it to the
`RustDesk-host=<id>,key=<base64>,api=<api>,relay=<relay>.exe` form that
`rustdesk/src/custom_server.rs:39` (`get_custom_server_from_string`) is able
to parse on first launch.

## How to populate

1. Download an official RustDesk Windows portable EXE
   (https://github.com/rustdesk/rustdesk/releases).
2. Compute its sha256:
   ```bash
   shasum -a 256 ./rustdesk-1.4.2-portable.exe
   ```
3. Upload it via the admin UI / `POST /api/admin/client_builder/base/upload`,
   supplying `source=upload`, `sha256=<hex>`, `version=1.4.2` and the file.

The API will move the file to `<sha256>.exe` inside this directory; the DB
row stores only metadata (name / sha / size / version / local path). Files
are **gitignored** by default (`*.exe`).

## Operations

- Cleanup (frees disk):  `rm -rf data/client-builder/base/*.exe`
- Cache flush (revokes outstanding tokens, optional):  restart the API
  process (memory cache) or `redis-cli --scan --pattern 'client_builder:*'
  | xargs redis-cli del` for Redis backends.

See `docs/operations/client-builder.md` for the full operator runbook.
