FROM alpine:3.20

RUN apk add --no-cache openssh-client rsync

COPY dist/linux-amd64/autobackup /usr/local/bin/autobackup
COPY e2e/config/autobackup.config.json /app/autobackup.config.json
COPY e2e/ssh/id_ed25519 /app/ssh/id_ed25519

RUN chmod 755 /usr/local/bin/autobackup \
    && chmod 600 /app/ssh/id_ed25519

WORKDIR /app

ENTRYPOINT ["/usr/local/bin/autobackup"]
CMD ["--config", "/app/autobackup.config.json", "--jobs", "1"]
