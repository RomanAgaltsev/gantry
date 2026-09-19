#!/bin/sh
# Start dockerd in the background and sshd in the foreground.
set -e

PUBKEY=/run/secrets/app_ssh_key.pub
if [ ! -f "$PUBKEY" ]; then
	echo "entrypoint: $PUBKEY is missing -- run stand/setup.sh first" >&2
	exit 1
fi

# Authorize the demo key for `deploy`.
mkdir -p /home/deploy/.ssh
cp "$PUBKEY" /home/deploy/.ssh/authorized_keys
chmod 700 /home/deploy/.ssh
chmod 600 /home/deploy/.ssh/authorized_keys
chown -R deploy:deploy /home/deploy/.ssh

# The project directory gantry deploys into. Seeded once: a redeploy must not
# clobber a compose file an operator edited by hand mid-demo.
mkdir -p /opt/demo
[ -f /opt/demo/compose.yaml ] || cp /opt/demo-seed/compose.yaml /opt/demo/compose.yaml
chown -R deploy:deploy /opt/demo

ssh-keygen -A

# dind ships TLS on by default, which would need certs distributed to a client
# that only ever talks over the local socket. The socket is the whole interface
# here, so TLS is off and the socket is handed to `deploy` via the docker group.
export DOCKER_TLS_CERTDIR=""
dockerd-entrypoint.sh dockerd --host=unix:///var/run/docker.sock >/var/log/dockerd.log 2>&1 &

i=0
while [ ! -S /var/run/docker.sock ]; do
	i=$((i + 1))
	if [ "$i" -gt 60 ]; then
		echo "entrypoint: dockerd did not create its socket in 60s" >&2
		tail -30 /var/log/dockerd.log >&2 || true
		exit 1
	fi
	sleep 1
done
addgroup deploy docker 2>/dev/null || true
chown root:docker /var/run/docker.sock
chmod 660 /var/run/docker.sock

echo "entrypoint: dockerd up, starting sshd"
exec /usr/sbin/sshd -D -e
