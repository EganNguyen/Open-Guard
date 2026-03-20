#!/usr/bin/env bash
set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")/../infra/certs" && pwd)"
mkdir -p "$CERT_DIR"

echo "==> Generating mTLS certificates in $CERT_DIR"

# CA (only if not already present)
if [ ! -f "$CERT_DIR/ca.crt" ]; then
  echo "--- Generating CA..."
  openssl req -x509 -sha256 -nodes -days 3650 \
    -newkey rsa:4096 \
    -keyout "$CERT_DIR/ca.key" \
    -out "$CERT_DIR/ca.crt" \
    -subj "/C=US/ST=State/L=City/O=OpenGuard/OU=Root/CN=OpenGuard-Root-CA" \
    2>/dev/null
else
  echo "--- CA already exists, skipping."
fi

generate_service_cert() {
  local svc="$1"
  shift
  local sans="$*"

  if [ -f "$CERT_DIR/${svc}.crt" ]; then
    echo "--- ${svc} cert already exists, skipping."
    return
  fi

  echo "--- Generating ${svc} cert (SANs: ${sans})..."
  # Write SAN extension file
  echo "subjectAltName=${sans}" > "$CERT_DIR/${svc}.ext"

  # Generate key + CSR
  openssl req -nodes -newkey rsa:2048 \
    -keyout "$CERT_DIR/${svc}.key" \
    -out "$CERT_DIR/${svc}.csr" \
    -subj "/C=US/ST=State/L=City/O=OpenGuard/OU=Services/CN=${svc}" \
    2>/dev/null

  # Sign with CA
  openssl x509 -req \
    -in "$CERT_DIR/${svc}.csr" \
    -CA "$CERT_DIR/ca.crt" \
    -CAkey "$CERT_DIR/ca.key" \
    -CAcreateserial \
    -out "$CERT_DIR/${svc}.crt" \
    -days 365 -sha256 \
    -extfile "$CERT_DIR/${svc}.ext" \
    2>/dev/null

  # Cleanup temp files
  rm -f "$CERT_DIR/${svc}.csr" "$CERT_DIR/${svc}.ext"
}

generate_service_cert "gateway" "DNS:gateway,DNS:localhost"
generate_service_cert "iam"     "DNS:iam,DNS:localhost"
generate_service_cert "policy"  "DNS:policy,DNS:localhost"

echo "==> Certificates ready in $CERT_DIR"
ls -la "$CERT_DIR"/*.crt "$CERT_DIR"/*.key
