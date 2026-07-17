# autobackup

`autobackup` is a minimal, fully functional v0.1 backup CLI. It is a
portable `rsync` over SSH runner for machines that need predictable incremental
copies to a remote host without adopting a full archive/repository backup
system.

The initial design target is a Windows/Linux workstation backing up large,
messy user trees to an SSH server:

- millions of small audio/sample-pack files in nested DAW folders
- `.git` checkouts and other VCS-heavy trees
- large download folders containing multi-GB files
- paths with spaces and Windows drive paths

The destination is a normal remote directory tree. `autobackup` does not create
a proprietary backup repository.

## Current Features

- JSON configuration with multiple source locations.
- Remote destination over SSH using `rsync`.
- Linux and Windows amd64 builds.
- Optional bundled `rsync` and `ssh` lookup beside the executable, with `PATH`
  fallback.
- Windows source path conversion for MSYS2/Cygwin/native rsync builds.
- `--dry-run` support.
- Quiet mode for heartbeat-only progress plus final summaries.
- Per-location `--delete`.
- Include pattern support.
- Prefix and substring excludes.
- Concurrent rsync task execution through `jobs`.
- Adaptive automatic task planning:
  - top-level directories are split into parallel rsync tasks when safe
  - source-root files get a dedicated root-files task
  - VCS-heavy or very deep top-level folders are isolated into recursive rsync
    tasks while siblings can still be split
- Docker end-to-end test stack using a real SSH server and rsync.

## Requirements

Local machine:

- Go, for building from source
- `rsync` and `ssh`, either on `PATH` or bundled beside the executable

Remote machine:

- SSH server
- `rsync` available for the remote user
- a writable destination base directory

Windows portability is a project requirement. Windows builds are expected to
work with rsync distributions such as MSYS2/Cygwin-style packages when the
configured or bundled tool paths point at compatible binaries.

## Windows Setup

Install [MSYS2](https://www.msys2.org/docs/installer/), then install the tools:
`pacman -S rsync openssh`.
Default paths are usually `C:\msys64\usr\bin\rsync.exe` and
`C:\msys64\usr\bin\ssh.exe`.
Other Windows ports of `rsync` and `ssh` should also work.

## Build

Build both supported targets:

```sh
scripts/build-all.sh
```

Build one target:

```sh
scripts/build-linux-amd64.sh
scripts/build-windows-amd64.sh
```

Build artifacts are written to:

- `dist/linux-amd64/autobackup`
- `dist/windows-amd64/autobackup.exe`

The `dist/` directory is generated output and is ignored by Git.

At runtime, the executable checks for bundled tools before falling back to
`PATH`:

- `tools/linux-amd64/rsync`
- `tools/linux-amd64/ssh`
- `tools/windows-amd64/rsync.exe`
- `tools/windows-amd64/ssh.exe`

## Test

Run unit tests:

```sh
go test ./...
```

Run the Docker end-to-end stack:

```sh
scripts/e2e-docker.sh
```

The Docker test builds the Linux binary, starts an SSH server with `rsync`, runs
`autobackup` from another container, and verifies the copied files and excludes.
More detail is in [e2e/README.md](e2e/README.md).

## Run

```sh
autobackup --config backup.json
autobackup --config backup.json --dry-run
autobackup --config backup.json --jobs 16
autobackup --config backup.json --quiet
autobackup --version
```

If `--config` is omitted, `AUTO_BACKUP_CONFIG` is used when set. Otherwise the
executable looks for `autobackup.config.json` in the current directory first,
then beside itself.

## Config

Configuration files use JSON with dashed field names. Unknown fields are
ignored by the JSON decoder. A sanitized starting point is available in
[autobackup.config.example.json](autobackup.config.example.json). The binary is
portable and can use any config path. This repository ignores `local-config/`
for developer-machine configs.

```json
{
  "destination": {
    "host": "10.0.0.100",
    "username": "pi",
    "base-path": "/tmp/disk/backup",
    "identity-file": "/path/to/id_rsa"
  },
  "tools": {
    "rsync": "/usr/bin/rsync",
    "ssh": "/usr/bin/ssh"
  },
  "jobs": 16,
  "quiet": false,
  "windows-path-style": "auto",
  "locations": [
    {
      "source": "C:\\home",
      "destination": "home",
      "parallel-rsync": false,
      "pattern": "**",
      "verification": "audit",
      "exclude-prefixes": ["Downloads"],
      "exclude-strings": [".venv"],
      "delete": false
    }
  ]
}
```

Required fields:

- `destination.host`
- `destination.username`
- `destination.base-path`
- at least one `locations` entry
- each location's `source` and `destination`

Important fields:

- `destination.base-path` is the remote root under which all location
  destinations are created.
- `destination.identity-file` enables SSH batch mode and disables password
  prompts.
- `tools.rsync` and `tools.ssh` override tool discovery.
- `jobs` controls concurrent rsync tasks and defaults to `16`.
- `quiet` hides normal per-task logs and only prints periodic heartbeats,
  final summaries, and errors.
- `windows-path-style` can be `auto`, `native`, `msys`, or `cygwin`.
- `delete` defaults to `false`; set it per location to pass `--delete`.
- `pattern` defaults to `**`; use patterns such as `*.pdf` to include only
  matching files while keeping directories.
- `verification` defaults to `audit`; use `changed`, `audit`, or `full`.
- `exclude-prefixes` excludes source-relative path roots.
- `exclude-strings` excludes paths containing a source-relative substring.
- `parallel-rsync` is optional:
  - omitted: use the adaptive automatic planning heuristic
  - `false`: use one root rsync task
  - `true`: create directory-level rsync tasks

On Windows, `autobackup` runs a best-effort `icacls` repair before connecting
when `destination.identity-file` is set, so OpenSSH is more likely to accept
private keys that were previously readable by the Windows Users group.

## Verification And Speed

`verification` is configured per location:

- `changed`: keep the normal rsync transfer behavior, with no post-transfer
  checksum pass.
- `audit`: default. After rsync completes, select a bounded daily sample of
  regular files from the task and run an rsync checksum dry-run for those paths.
- `full`: after rsync completes, run an rsync checksum dry-run for the full
  task.

The normal transfer path uses rsync's quick-check behavior for speed. Audit and
full verification add checksum dry-runs after transfer without keeping a local
manifest or cache.

The automatic planner splits top-level folders where it can. Source-root files
get a root-files task, while deep or VCS-heavy top-level folders are kept in
recursive rsync tasks so the rest of the location can still run in parallel.

## Code Structure

- [cmd/autobackup/main.go](cmd/autobackup/main.go): CLI flags, config loading,
  plan creation, signal handling, exit codes.
- [internal/autobackup/config.go](internal/autobackup/config.go): JSON config
  schema, defaults, validation, config path resolution.
- [internal/autobackup/plan.go](internal/autobackup/plan.go): tool resolution,
  folder discovery, parallelization heuristic, rsync/SSH argument construction.
- [internal/autobackup/run.go](internal/autobackup/run.go): worker pool,
  command execution, rsync output parsing, summary reporting.
- [internal/autobackup/paths.go](internal/autobackup/paths.go): platform tags,
  bundled tool lookup, Windows path conversion, remote path joining.
- `internal/autobackup/identity_*.go`: platform-specific SSH identity-file
  preparation.
- `internal/autobackup/process_*.go`: platform-specific process termination.
- `internal/autobackup/*_test.go`: unit tests for config, planning, paths, and
  runner behavior.
- `e2e/`: Docker-based end-to-end test fixture and SSH server.
- `scripts/`: build and test helper scripts.

## Docker E2E

Run the Docker-based end-to-end stack from the repository root:

```sh
scripts/e2e-docker.sh
```

It builds the Linux binary, starts an SSH server with `rsync`, runs
`autobackup` from a separate container with fixture files, and verifies the
files landed in the remote backup volume.
