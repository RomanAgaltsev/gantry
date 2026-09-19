# The demo app-host: a Docker-in-Docker daemon reachable over SSH, which is the
# shape gantry's compose-over-ssh executor expects of a real deployment target.
FROM docker:27-dind

# Only sshd is missing from the base. `docker compose` (the v2 plugin the verifier
# shells out to in internal/verify/host.go) ships with docker:27-dind already --
# verified at v2.33.0 -- so installing docker-cli-compose would be redundant.
#
# --upgrade is required, not cosmetic: the base pins openssh-client 9.9_p1-r2 while
# the Alpine 3.21 index now carries p2, and a plain `apk add openssh` fails to
# resolve against the installed p1. Upgrading moves the whole openssh set together.
RUN apk add --no-cache --upgrade openssh

# gantry connects as `deploy`, because examples/demo/gantry.yaml says so and that
# file is the artifact under proof. `passwd -d` leaves the account unlocked with no
# password: sshd refuses password auth (PermitEmptyPasswords defaults to no) while
# public-key auth still works.
RUN adduser -D -s /bin/sh deploy \
    && passwd -d deploy \
    && (getent group docker >/dev/null || addgroup -S docker)

COPY compose.yaml /opt/demo-seed/compose.yaml
COPY entrypoint.sh /usr/local/bin/stand-entrypoint.sh
RUN chmod +x /usr/local/bin/stand-entrypoint.sh

EXPOSE 22
ENTRYPOINT ["/usr/local/bin/stand-entrypoint.sh"]
