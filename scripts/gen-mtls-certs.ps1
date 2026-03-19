$ErrorActionPreference = "Stop"

$CertPath = Join-Path $PWD "infra/certs"
if (-not (Test-Path $CertPath)) {
    New-Item -ItemType Directory -Force -Path $CertPath | Out-Null
}

Write-Host "Generating certificates using Docker (alpine/openssl)..."

docker run --rm -v "$($CertPath):/certs" -w /certs alpine/openssl req -x509 -sha256 -nodes -days 3650 -newkey rsa:4096 -keyout ca.key -out ca.crt -subj "/C=US/ST=State/L=City/O=OpenGuard/OU=Root/CN=OpenGuard-Root-CA"

docker run --rm -v "$($CertPath):/certs" -w /certs alpine/openssl sh -c "echo 'subjectAltName=DNS:gateway,DNS:localhost' > gateway.ext && openssl req -nodes -newkey rsa:2048 -keyout gateway.key -out gateway.csr -subj '/C=US/ST=State/L=City/O=OpenGuard/OU=Edge/CN=gateway' && openssl x509 -req -in gateway.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out gateway.crt -days 365 -sha256 -extfile gateway.ext"

docker run --rm -v "$($CertPath):/certs" -w /certs alpine/openssl sh -c "echo 'subjectAltName=DNS:iam,DNS:localhost' > iam.ext && openssl req -nodes -newkey rsa:2048 -keyout iam.key -out iam.csr -subj '/C=US/ST=State/L=City/O=OpenGuard/OU=Services/CN=iam' && openssl x509 -req -in iam.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out iam.crt -days 365 -sha256 -extfile iam.ext"

Write-Host "Certificates successfully generated in infra/certs!"
