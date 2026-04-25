#!/bin/bash
set -e
echo "Initializing LocalStack services..."

# Create S3 bucket for compliance reports
awslocal s3 mb s3://compliance-reports

# Create secrets for various services
echo "Creating secrets..."

awslocal secretsmanager create-secret \
    --name IAM_JWT_KEYS \
    --secret-string '[{"kid":"v1","secret":"SyCVT3SHMQqd3Wp9+u2llGVurh3TcgOtOrTRbeqzfEIzrlpgS9FNB5SrwyBQIdMd7na9yT5fyV8vp8Wcm9xHlA==","algorithm":"HS256","status":"active"}]'

awslocal secretsmanager create-secret \
    --name IAM_MFA_BACKUP_CODE_HMAC_SECRET \
    --secret-string '6053f7f3a1ada74c803980a2aafbcf820ee7fde32c1eedbc1067190326ed6d46'

awslocal secretsmanager create-secret \
    --name AUDIT_SECRET_KEY \
    --secret-string '0df35ceee02ae12d399e33a8d0d0d811c3d54d3d06bf3e6292a00f66886cea45'

awslocal secretsmanager create-secret \
    --name ALERTING_SIEM_WEBHOOK_HMAC_SECRET \
    --secret-string '81c62ac76fce3b3cd2f7461ab3a5ca58206e911b75bf252399a20632905b186a'

awslocal secretsmanager create-secret \
    --name IAM_AES_KEYS \
    --secret-string '[{"kid":"v1","key":"w12OXycFaefbju9Om23FII73pDoev8OJaEO2Ug7UhDc=","status":"active"}]'

echo "LocalStack initialization complete."
