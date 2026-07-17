# Docker E2E Stack

This directory contains a Docker-based end-to-end test for `autobackup`.
It proves the compiled Linux binary can connect to an SSH server, run `rsync`
over SSH, write into a remote backup volume, and produce the expected output.

## Quick Start

Run from the repository root:

```sh
scripts/e2e-docker.sh
```

Expected final output:

```text
docker e2e passed
```

The script is destructive only to its own Docker Compose project. It removes
and recreates the `autobackup-e2e` containers, network, and named volume on
each run.

## Requirements

- Docker with the Compose plugin: `docker compose version`
- Go available on `PATH`, unless you run the script with a shell `PATH` that
  points at a local Go install
- Network access the first time Alpine base images and packages are pulled

If Docker is available through a non-default context, set `DOCKER_CONTEXT`:

```sh
DOCKER_CONTEXT=desktop-linux scripts/e2e-docker.sh
```

## Stack Layout

- `docker-compose.e2e.yml` defines the test stack.
- `e2e/ssh-server/Dockerfile` builds the fake remote backup host.
- `e2e/runner/Dockerfile` builds the container that runs `autobackup`.
- `e2e/config/autobackup.config.json` is the test config consumed by the
  runner container.
- `e2e/source/` contains fixture files mounted read-only into the runner.
- `e2e/ssh/id_ed25519` and `.pub` are generated throwaway test credentials
  used only by this stack.
- `scripts/e2e-docker.sh` builds, runs, verifies, and cleans up the stack.

## How It Works

The script first generates a local throwaway SSH keypair under `e2e/ssh/` when
one does not already exist, then builds the Linux binary:

```sh
scripts/build-linux-amd64.sh
```

It then runs:

```sh
docker compose -f docker-compose.e2e.yml -p autobackup-e2e down -v --remove-orphans
docker compose -f docker-compose.e2e.yml -p autobackup-e2e build
docker compose -f docker-compose.e2e.yml -p autobackup-e2e up -d ssh-server
docker compose -f docker-compose.e2e.yml -p autobackup-e2e run --rm autobackup
```

The `ssh-server` service is an Alpine container with:

- `openssh`
- `rsync`
- user `backup`
- public-key auth using `e2e/ssh/id_ed25519.pub`
- backup volume mounted at `/tmp/disk/backup root`

The `autobackup` service is an Alpine container with:

- `openssh-client`
- `rsync`
- `dist/linux-amd64/autobackup`
- test config copied to `/app/autobackup.config.json`
- private key copied to `/app/ssh/id_ed25519`
- fixture source mounted at `/data/source`

The test config backs up `/data/source` to a destination that intentionally
contains spaces:

```text
backup@ssh-server:/tmp/disk/backup root/e2e host
```

After the backup run, the script starts a short-lived verification container
from the `ssh-server` image and checks that these files exist in the named
volume with the expected contents:

- `/tmp/disk/backup root/e2e host/root-note.txt`
- `/tmp/disk/backup root/e2e host/top level space file.txt`
- `/tmp/disk/backup root/e2e host/docs/report.txt`
- `/tmp/disk/backup root/e2e host/docs/folder with spaces/nested note.txt`
- `/tmp/disk/backup root/e2e host/photos/image-001.txt`

Finally it runs `down -v --remove-orphans` again so the test leaves no running
containers or named volumes behind.

## Manual Commands

To inspect the stack without automatic cleanup:

```sh
mkdir -p e2e/ssh
ssh-keygen -q -t ed25519 -N "" -C "autobackup-e2e" -f e2e/ssh/id_ed25519
scripts/build-linux-amd64.sh
docker compose -f docker-compose.e2e.yml -p autobackup-e2e build
docker compose -f docker-compose.e2e.yml -p autobackup-e2e up -d ssh-server
docker compose -f docker-compose.e2e.yml -p autobackup-e2e run --rm autobackup
docker compose -f docker-compose.e2e.yml -p autobackup-e2e logs ssh-server
```

Inspect copied files:

```sh
docker compose -f docker-compose.e2e.yml -p autobackup-e2e run --rm --no-deps --entrypoint sh ssh-server
ls -R "/tmp/disk/backup root"
```

Clean up:

```sh
docker compose -f docker-compose.e2e.yml -p autobackup-e2e down -v --remove-orphans
```

## Common Failures

`permission denied while trying to connect to the docker API`

The current shell cannot access the Docker daemon. Start Docker Desktop,
enable integration for this WSL distro, or set `DOCKER_CONTEXT` to a working
context.

`rsync: command not found`

Both sides of rsync-over-SSH need `rsync`. The provided Dockerfiles install it
in both containers; this error usually means a custom image was changed.

`Permission denied (publickey,keyboard-interactive)`

The test key and server account are out of sync. Rebuild the stack with:

```sh
docker compose -f docker-compose.e2e.yml -p autobackup-e2e build --no-cache
```

## Security Notes

The SSH key under `e2e/ssh/` is generated locally, ignored by Git, and used
only by the Docker e2e stack. Do not reuse it for real backups. The e2e SSH
server also relaxes host-key behavior through the normal `autobackup` SSH
options, matching the current CLI defaults.
