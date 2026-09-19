#!/bin/sh
# Generate everything examples/demo/gantry.yaml expects to find, so the shipped
# config's literal paths and addresses are all true.
#
# Idempotent: re-running does not regenerate a working stand's credentials. The one
# exception is known_hosts, which is rewritten on every run because app-host makes
# fresh host keys each time its container is created.
set -e

cd "$(dirname "$0")"

# openssl -subj arguments look like paths to MSYS, which rewrites them on Windows.
MSYS_NO_PATHCONV=1
export MSYS_NO_PATHCONV

SECRETS=./secrets
WORK=./.work
mkdir -p "$SECRETS"

# 1. The SSH keypair app-host authorizes and gantry connects with.
if [ ! -f "$SECRETS/app_ssh_key" ]; then
	echo "setup: generating the app-host ssh keypair"
	ssh-keygen -t ed25519 -N "" -C "gantry demo stand" -f "$SECRETS/app_ssh_key" >/dev/null
fi

# 2. A demo CA and a server certificate for gitlab.example.com.
#
# The certificate MUST carry a subjectAltName. Go's verifier is gantry's verifier,
# and it rejects a CN-only certificate outright: "x509: certificate relies on legacy
# Common Name field, use SANs instead". That is the single most likely thing to get
# wrong here, so it is asserted at the end of this step.
if [ ! -f "$SECRETS/forge.crt" ]; then
	echo "setup: generating the demo CA and the forge certificate"
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-keyout "$SECRETS/demo-ca.key" -out "$SECRETS/demo-ca.crt" \
		-subj "/CN=gantry demo CA" -days 365 2>/dev/null
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
		-keyout "$SECRETS/forge.key" -out "$SECRETS/forge.csr" \
		-subj "/CN=gitlab.example.com" 2>/dev/null
	printf 'subjectAltName=DNS:gitlab.example.com\nbasicConstraints=CA:FALSE\nkeyUsage=digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\n' \
		> "$SECRETS/forge.ext"
	openssl x509 -req -in "$SECRETS/forge.csr" \
		-CA "$SECRETS/demo-ca.crt" -CAkey "$SECRETS/demo-ca.key" -CAcreateserial \
		-out "$SECRETS/forge.crt" -days 365 -extfile "$SECRETS/forge.ext" 2>/dev/null
	# The forge runs as distroless nonroot (uid 65532) and reads the key from this
	# read-only mount. It is a throwaway demo CA, not a secret worth protecting.
	chmod 644 "$SECRETS/forge.key"

	if ! openssl x509 -in "$SECRETS/forge.crt" -noout -ext subjectAltName 2>/dev/null | grep -q gitlab.example.com; then
		echo "setup: the forge certificate has no subjectAltName; Go would reject it" >&2
		exit 1
	fi
fi

# 3. The scratch git tree gantry commits pin files into.
#
# gantry runs with working_dir /work, which is this directory. It must never be your
# gantry checkout, or a demo run would commit pin files into the repository.
if [ ! -d "$WORK/.git" ]; then
	echo "setup: creating the scratch git tree"
	mkdir -p "$WORK"
	git -C "$WORK" init -q
	git -C "$WORK" config user.name "gantry demo"
	git -C "$WORK" config user.email "demo@example.invalid"
	# Pin files are dotenv files read by `docker compose --env-file` on Linux. With
	# a global core.autocrlf=true -- the Windows default -- git would rewrite them
	# with CRLF, and every value would carry a trailing carriage return.
	git -C "$WORK" config core.autocrlf false
	# gantry.yaml is bind-mounted in from examples/demo/, and .gantry/ holds the
	# serve lock. Neither belongs in the scratch tree's history.
	printf 'gantry.yaml\n.gantry/\n' > "$WORK/.gitignore"
	printf '# gantry demo scratch tree\n\nPin files committed by demo runs land here.\n' > "$WORK/README.md"
	git -C "$WORK" add .gitignore README.md

	# POSTGRES_IMAGE is an explicit pin: examples/demo/gantry.yaml declares
	# `source: { pin: explicit }` for it, so gantry never asks the forge and never
	# writes it. It is the operator's to set, and without it the very first sync
	# fails with "service postgres has neither an image nor a build context".
	printf 'POSTGRES_IMAGE=postgres:16.4\n' > "$WORK/.env.versions.test"
	git -C "$WORK" add .env.versions.test
	git -C "$WORK" commit -q -m "initial"
fi

# 4. known_hosts for 192.0.2.10.
#
# Rewritten every run: app-host generates host keys at container start, so they
# change whenever the stand is reset. The host key is read from inside the
# container because 192.0.2.10 exists only on the compose network -- the address
# is then substituted in, because that is what the shipped config connects to.
echo "setup: starting app-host to read its host key"
docker compose up -d --build app-host >/dev/null

i=0
while :; do
	KEYS=$(docker compose exec -T app-host ssh-keyscan 127.0.0.1 2>/dev/null || true)
	if [ -n "$KEYS" ]; then
		break
	fi
	i=$((i + 1))
	if [ "$i" -gt 60 ]; then
		echo "setup: app-host sshd did not answer within 60s" >&2
		docker compose logs app-host >&2
		exit 1
	fi
	sleep 1
done
echo "$KEYS" | sed 's/^127\.0\.0\.1/192.0.2.10/' > "$SECRETS/known_hosts"

echo "setup: ready"
