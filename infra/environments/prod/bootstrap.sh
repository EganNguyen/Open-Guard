#!/bin/bash
set -e

# OpenGuard Production Bootstrap Script
# This script sets up the Terraform Backend (S3/DynamoDB) and initializes the production environment.

# Load environment variables from .env if it exists
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Ensure credentials are set for this script scope
export AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-test}
export AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-test}
export AWS_DEFAULT_REGION=${1:-"us-east-1"}

REGION=$AWS_DEFAULT_REGION
ENV=${2:-"prod"}
BUCKET_NAME="openguard-terraform-state-${ENV}"
TABLE_NAME="openguard-terraform-locks-${ENV}"

echo "🚀 Bootstrapping OpenGuard Production Environment ($ENV)..."

# LocalStack detection
ENDPOINT_OPT=""
CREATE_BUCKET_CFG=""
if [ "$INFRA_MODE" == "localstack" ]; then
    echo "☁️  LocalStack mode detected..."
    ENDPOINT_OPT="--endpoint-url http://localhost:4566"
    # LocalStack us-east-1 explicitly rejects LocationConstraint
    if [ "$REGION" != "us-east-1" ]; then
        CREATE_BUCKET_CFG="--create-bucket-configuration LocationConstraint=$REGION"
    fi
else
     if [ "$REGION" != "us-east-1" ]; then
        CREATE_BUCKET_CFG="--create-bucket-configuration LocationConstraint=$REGION"
    fi
fi

# Ensure directory exists for localstack environment if it's dynamic
mkdir -p "infra/environments/$ENV"

# 1. Create S3 Bucket for State
aws $ENDPOINT_OPT s3api create-bucket --bucket "$BUCKET_NAME" --region "$REGION" $CREATE_BUCKET_CFG || true
aws $ENDPOINT_OPT s3api put-bucket-versioning --bucket "$BUCKET_NAME" --versioning-configuration Status=Enabled || true

# 2. Create DynamoDB Table for Locking
aws $ENDPOINT_OPT dynamodb create-table \
    --table-name "$TABLE_NAME" \
    --attribute-definitions AttributeName=LockID,AttributeType=S \
    --key-schema AttributeName=LockID,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
    --region "$REGION" || true

# 3. Create backend.tf
cat <<EOF > "infra/environments/$ENV/backend.tf"
terraform {
  backend "s3" {
    bucket         = "$BUCKET_NAME"
    key            = "openguard/$ENV/terraform.tfstate"
    region         = "$REGION"
    dynamodb_table = "$TABLE_NAME"
    encrypt        = true
    ${INFRA_MODE:+"skip_credentials_validation = true"}
    ${INFRA_MODE:+"skip_metadata_api_check     = true"}
    ${INFRA_MODE:+"force_path_style            = true"}
    ${INFRA_MODE:+"endpoints = { s3 = \"http://localhost:4566\", dynamodb = \"http://localhost:4566\" }"}
  }
}
EOF

echo "✅ Bootstrap complete. Backend configured in infra/environments/$ENV/backend.tf"

# 4. LocalStack Specific: Sync Certs to Secrets Manager if in LocalStack mode
if [ "$INFRA_MODE" == "localstack" ]; then
    echo "🔐 Syncing mTLS certificates to LocalStack Secrets Manager..."
    
    # Upload CA and microservice certs
    aws $ENDPOINT_OPT secretsmanager create-secret --name openguard/mtls/ca-cert --secret-string "$(cat infra/certs/ca.crt)" || \
    aws $ENDPOINT_OPT secretsmanager update-secret --secret-id openguard/mtls/ca-cert --secret-string "$(cat infra/certs/ca.crt)"
    
    aws $ENDPOINT_OPT secretsmanager create-secret --name openguard/mtls/iam-cert --secret-string "$(cat infra/certs/iam/server.crt)" || \
    aws $ENDPOINT_OPT secretsmanager update-secret --secret-id openguard/mtls/iam-cert --secret-string "$(cat infra/certs/iam/server.crt)"
fi
