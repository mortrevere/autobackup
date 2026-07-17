#!/usr/bin/env sh
set -eu

ssh-keygen -A
mkdir -p /run/sshd "/tmp/disk/backup root"
chown -R backup:backup "/tmp/disk/backup root"
exec /usr/sbin/sshd -D -e
