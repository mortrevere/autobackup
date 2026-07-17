# autobackup

`autobackup` copies local folders to a remote machine over SSH using `rsync`.
It is meant for straightforward incremental backups to a normal directory tree:
no proprietary archive format, no repository database, and no restore tool
required.

It is a good fit for workstations with large, messy folders: documents,
projects, downloads, media libraries, DAW sample packs, `.git` trees, etc.

## What It Does

- Backs up one or more local locations to an SSH host.
- Stores files under a normal remote directory.
- Uses `rsync` for incremental copies.
- Can run a dry run before changing the destination.
- Can skip paths by prefix or substring.
- Can include only files matching a pattern.
- Can either append/update files or mirror deletions with per-location
  `delete`.
- Runs multiple rsync jobs when a location can be split safely.
- Supports Linux and Windows amd64 builds.

## Requirements

On the machine running `autobackup`:

- `rsync`
- `ssh`
- a JSON config file (which can be generated interactively)

On the destination machine:

- SSH server
- `rsync`
- a writable backup directory for the SSH user

`autobackup` looks for `rsync` and `ssh` beside the executable first, then on
`PATH`. You can also set exact tool paths in the config.

## Windows Setup

Install [MSYS2](https://www.msys2.org/docs/installer/), then install the tools:

```sh
pacman -S rsync openssh
```

The common MSYS2 paths are:

- `C:\msys64\usr\bin\rsync.exe`
- `C:\msys64\usr\bin\ssh.exe`

If `destination.identity-file` is set, `autobackup` also does a best-effort
Windows key-permission repair before connecting, so OpenSSH is more likely to
accept private keys that were previously readable by the Windows Users group.

## Create A Config

The easiest starting point is:

```sh
autobackup -init
```

`-init` first asks whether you want to configure interactively.

If you answer yes, it asks for:

- destination host
- destination username
- destination base path
- SSH key location
- first source path
- remote destination folder for that source
- include pattern
- one optional exclusion
- whether the destination should append/update only or sync deletions too

It writes a valid JSON config and prints the absolute path.

If you answer no, it immediately writes a valid default config and prints its
absolute path. Edit that file before running a real backup.

You can choose where the file is written:

```sh
autobackup -config /path/to/autobackup.config.json -init
```

`-init` refuses to overwrite an existing config.

## Run A Backup

Start with a dry run:

```sh
autobackup -config /path/to/autobackup.config.json -dry-run
```

Run the backup:

```sh
autobackup -config /path/to/autobackup.config.json
```

Useful flags:

```sh
autobackup -config backup.json -jobs 8
autobackup -config backup.json -quiet
autobackup -version
autobackup -help
```

If `-config` is omitted, `AUTO_BACKUP_CONFIG` is used when set. Otherwise
`autobackup` looks for `autobackup.config.json` in the current directory first,
then beside the executable.

## Config Example

Configuration files are JSON:

```json
{
  "destination": {
    "host": "backup.example.com",
    "username": "backup",
    "base-path": "/srv/backups/workstation",
    "identity-file": "/home/me/.ssh/id_ed25519"
  },
  "locations": [
    {
      "source": "/home/me/Documents",
      "destination": "documents",
      "pattern": "**",
      "verification": "audit",
      "exclude-prefixes": [
        "Downloads"
      ],
      "exclude-strings": [
        ".venv",
        ".git"
      ],
      "delete": false
    }
  ],
  "tools": {
    "rsync": "",
    "ssh": ""
  },
  "windows-path-style": "auto",
  "jobs": 16,
  "quiet": false
}
```

Another example is available in
[autobackup.config.example.json](autobackup.config.example.json).

## Config Keys

Required keys:

- `destination.host`: remote SSH host.
- `destination.username`: remote SSH username.
- `destination.base-path`: remote root under which backups are created.
- `locations[].source`: local source path to back up.
- `locations[].destination`: remote folder under `destination.base-path`.

Optional keys:

- `destination.identity-file`: SSH private key path. When set, SSH runs in
  batch mode and does not prompt for passwords.
- `tools.rsync`: exact local `rsync` path. Empty uses bundled tools, then
  `PATH`.
- `tools.ssh`: exact local `ssh` path. Empty uses bundled tools, then `PATH`.
- `jobs`: maximum concurrent rsync jobs. Default: `16`.
- `quiet`: only print heartbeats, final summaries, and errors.
- `windows-path-style`: `auto`, `native`, `msys`, or `cygwin`. Default:
  `auto`.
- `locations[].parallel-rsync`: optional planning override. Omit for automatic
  planning, use `false` for one root rsync task, or `true` for directory-level
  tasks.
- `locations[].pattern`: include pattern. Default: `**`.
- `locations[].verification`: `changed`, `audit`, or `full`. Default: `audit`.
- `locations[].exclude-prefixes`: source-relative prefixes to skip entirely.
- `locations[].exclude-strings`: source-relative path substrings to skip.
- `locations[].delete`: pass `--delete` to rsync for that location.

## Delete Behavior

By default, `delete` is `false`. That means `autobackup` copies new and changed
files but does not remove old files from the destination.

Set `delete` to `true` for a location when the remote folder should mirror the
source and remove files that were deleted locally.

## Verification

`verification` is configured per location:

- `changed`: normal rsync transfer behavior, with no post-transfer checksum
  pass.
- `audit`: default. After rsync completes, check a bounded daily sample of
  regular files with an rsync checksum dry run.
- `full`: after rsync completes, check the full task with an rsync checksum dry
  run.

Normal transfers use rsync's quick-check behavior for speed. `audit` and
`full` add checksum dry runs after transfer without keeping a local manifest or
cache.

## Performance

`autobackup` automatically plans rsync work for each location. Where safe, it
splits top-level folders into parallel jobs. Source-root files get their own
task. Very deep or VCS-heavy top-level folders are kept recursive so other
folders can still run in parallel.

Use `jobs` in the config or `-jobs` on the command line to control concurrency.

## Build From Source

Building from source requires Go.

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

The executable checks for bundled tools before falling back to `PATH`:

- `tools/linux-amd64/rsync`
- `tools/linux-amd64/ssh`
- `tools/windows-amd64/rsync.exe`
- `tools/windows-amd64/ssh.exe`

## Test

Run unit tests:

```sh
go test ./...
```

Run the Docker end-to-end test:

```sh
scripts/e2e-docker.sh
```

The Docker test builds the Linux binary, starts an SSH server with `rsync`, runs
`autobackup` from another container, and verifies copied files and excludes.
More detail is in [e2e/README.md](e2e/README.md).
