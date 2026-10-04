#!/usr/bin/env bash
# Local test CA and custody Secret only. Production uses managed Secret/PKI.
# Existing or partial material is NEVER overwritten or implicitly regenerated.
set -euo pipefail
umask 077
trap 'echo "Platform test material initialization failed; existing files preserved" >&2' ERR
if [[ $# != 1 || "$1" != /* || -e "$1" || -L "$1" ]]; then
  echo "Provide one new absolute destination directory; existing material is not overwritten" >&2
  exit 1
fi
destination="$1"
parent_directory="$(dirname "$destination")"
if [[ -L "$parent_directory" ]]; then
  echo "Unsafe material parent directory" >&2
  exit 1
fi
mkdir -p -m 700 "$parent_directory"
mkdir -m 700 "$destination"
openssl rand -out "$destination/custody-v1.key" 32
openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$destination/client-ca-key.pem" 2>/dev/null
openssl req -new -x509 -key "$destination/client-ca-key.pem" -out "$destination/client-ca.pem" -days 365 -sha256 \
  -subj '/CN=media-agent isolated Platform test CA' \
  -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign' 2>/dev/null
for name in server gateway schedule-worker; do
  openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$destination/$name-key.pem" 2>/dev/null
  if [[ "$name" == server ]]; then
    identity='DNS:media_auth,DNS:localhost,IP:127.0.0.1'
    usage=serverAuth
  elif [[ "$name" == gateway ]]; then
    identity='URI:spiffe://media-agent/services/media_agent_gateway'
    usage=clientAuth
  else
    identity='URI:spiffe://media-agent/services/media_control_schedule_worker'
    usage=clientAuth
  fi
  openssl req -new -key "$destination/$name-key.pem" -out "$destination/$name.csr" -subj "/CN=$name" \
    -addext 'basicConstraints=critical,CA:FALSE' -addext 'keyUsage=critical,digitalSignature' \
    -addext "extendedKeyUsage=$usage" -addext "subjectAltName=$identity" 2>/dev/null
  openssl x509 -req -in "$destination/$name.csr" -CA "$destination/client-ca.pem" -CAkey "$destination/client-ca-key.pem" \
    -CAcreateserial -out "$destination/$name.pem" -days 90 -sha256 -copy_extensions copy 2>/dev/null
done
echo 'Platform test materials initialized; no credentials printed'
