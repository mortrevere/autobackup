FROM alpine:3.20

RUN apk add --no-cache openssh rsync

RUN adduser -D -h /home/backup backup \
    && passwd -d backup \
    && mkdir -p /home/backup/.ssh "/tmp/disk/backup root" \
    && chown -R backup:backup /home/backup /tmp/disk

COPY e2e/ssh/id_ed25519.pub /home/backup/.ssh/authorized_keys
COPY e2e/ssh-server/entrypoint.sh /usr/local/bin/e2e-sshd

RUN chmod 700 /home/backup/.ssh \
    && chmod 600 /home/backup/.ssh/authorized_keys \
    && chown backup:backup /home/backup/.ssh/authorized_keys \
    && chmod 755 /usr/local/bin/e2e-sshd \
    && printf '%s\n' \
        'PasswordAuthentication no' \
        'PermitRootLogin no' \
        'PubkeyAuthentication yes' \
        'AuthorizedKeysFile .ssh/authorized_keys' \
        >> /etc/ssh/sshd_config

EXPOSE 22

HEALTHCHECK --interval=1s --timeout=3s --retries=30 CMD ssh-keyscan -p 22 localhost >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/e2e-sshd"]
