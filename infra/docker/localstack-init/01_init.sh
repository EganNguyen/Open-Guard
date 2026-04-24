#!/bin/bash
echo "Initializing LocalStack services..."

# Create S3 bucket for compliance reports
awslocal s3 mb s3://compliance-reports

# Create secret for JWT keys (example)
awslocal secretsmanager create-secret \
    --name openguard/jwt-keys \
    --secret-string '[{"kid":"k1","secret":"super-secret-key-at-least-32-chars","algorithm":"HS256","status":"active"}]'

echo "LocalStack initialization complete."
