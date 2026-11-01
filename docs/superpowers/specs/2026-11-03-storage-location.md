# Where the database lives

Status: **implemented.**

SQLite is a file, and a file is only a database if the process can rely on
locking and in-place writes. A cloud sync daemon rewrites the same file while it
is open. That is not a supported configuration, even though it often works for
days.

## The symptom

The first report was an intermittent:

> Storage operation failed; session stopped. Check storage and retry recovery.

Recovery worked, then the failure returned. The database was
`~/Documents/Code/.../galzu.db`; this Mac syncs Desktop & Documents to iCloud
(`brctl status` reports `Desktop & Documents: current=YES`). SQLite errors from a
sync race look like transient I/O or lock failures, which is exactly why retrying
was misleading.

## The rule

- The default database is the user config directory:
  `~/Library/Application Support/Galzu/galzu.db` on macOS,
  `os.UserConfigDir()/Galzu/galzu.db` elsewhere.
- `-db` still selects any path. A path under Documents, Desktop, Dropbox,
  OneDrive, Google Drive or Mobile Documents produces a warning at startup and in
  the Sessions library; it is a warning, not a refusal, because the operator may
  know the folder is not actually synced.
- On first start with the default, if `./galzu.db` exists and the default does
  not, the old file is copied once. The original is left as a backup. A sidecar
  `-wal`, `-shm` or `-journal` stops the copy with instructions, because copying
  the main file alone could lose a transaction and silently starting empty would
  look like data loss.
- `PRAGMA busy_timeout=5000` waits through a short lock instead of failing at
  once. It does not make a synced folder safe.

## Diagnosis

A storage failure now includes the underlying error in the session banner and
the server log. The generic sentence told the operator to retry but not what to
fix.

## What this does not solve

- A database on a network filesystem, a VM shared folder or a fuse mount has the
  same class of problem; the warning covers common sync folders, not every
  filesystem.
- An existing database in a synced folder is not moved automatically. The copy
  happens only when the default is absent, and it is best done after the old
  server is stopped.
