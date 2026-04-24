#!/bin/bash
set -e

# MaxMind GeoLite2 City download script
# Requires MAXMIND_LICENSE_KEY env var

if [ -z "$MAXMIND_LICENSE_KEY" ]; then
    echo "Error: MAXMIND_LICENSE_KEY is not set."
    exit 1
fi

DB_DIR="./data"
mkdir -p "$DB_DIR"

URL="https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=${MAXMIND_LICENSE_KEY}&suffix=tar.gz"

echo "Downloading GeoLite2 City database..."
curl -sL "$URL" -o "${DB_DIR}/GeoLite2-City.tar.gz"

echo "Extracting..."
tar -xzf "${DB_DIR}/GeoLite2-City.tar.gz" -C "$DB_DIR" --strip-components=1

# Clean up
rm "${DB_DIR}/GeoLite2-City.tar.gz"
echo "GeoLite2 City database downloaded to ${DB_DIR}/GeoLite2-City.mmdb"
