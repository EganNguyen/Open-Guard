#!/bin/bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
OUT_DIR="$DIR/../infra/certs"

mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo "Generating mTLS CA..."
openssl req -x509 -newkey rsa:4096 -keyout ca.key -out ca.crt -days 3650 -nodes -subj "/CN=OpenGuard-CA"

echo "Generating Server/Client certificates..."
for service in iam policy control-plane gateway audit; do
    echo " -> $service"
    mkdir -p "$service"
    openssl req -newkey rsa:2048 -keyout ${service}/server.key -out ${service}/server.csr -nodes -subj "/CN=${service}"
    openssl x509 -req -in ${service}/server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out ${service}/server.crt -days 365
    # Also keep the named version for backwards compatibility if needed
    cp ${service}/server.crt ${service}.crt
    cp ${service}/server.key ${service}.key
done

echo "mTLS certs generated successfully in $OUT_DIR."
