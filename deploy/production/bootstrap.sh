#!/bin/bash
set -e

# OpenGuard Production Bootstrap Script
# This script sets up the Terraform Backend (S3/DynamoDB) and initializes the production environment.

REGION=${1:-"us-east-1"}
ENV=${2:-"prod"}
BUCKET_NAME="openguard-terraform-state-${ENV}-$(date +%s)"
TABLE_NAME="openguard-terraform-locks-${ENV}"

echo "🚀 Bootstrapping OpenGuard Production Environment..."

# 1. Create S3 Bucket for State
aws s3api create-bucket --bucket $BUCKET_NAME --region $REGION --create-bucket-configuration LocationConstraint=$REGION || true
aws s3api put-bucket-versioning --bucket $BUCKET_NAME --versioning-configuration Status=Enabled

# 2. Create DynamoDB Table for Locking
aws dynamodb create-table \
    --table-name $TABLE_NAME \
    --attribute-definitions AttributeName=LockID,AttributeType=S \
    --key-schema AttributeName=LockID,KeyType=HASH \
    --provisioned-throughput ReadCapacityUnits=5,WriteCapacityUnits=5 \
    --region $REGION || true

# 3. Create backend.tf
cat <<EOF > deploy/production/terraform/environments/$ENV/backend.tf
terraform {
  backend "s3" {
    bucket         = "$BUCKET_NAME"
    key            = "openguard/$ENV/terraform.tfstate"
    region         = "$REGION"
    dynamodb_table = "$TABLE_NAME"
    encrypt        = true
  }
}
EOF

echo "✅ Bootstrap complete. Backend configured in deploy/production/terraform/environments/$ENV/backend.tf"
echo "👉 Next: cd deploy/production/terraform/environments/$ENV && terraform init"
