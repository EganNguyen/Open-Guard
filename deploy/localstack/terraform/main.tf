terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  access_key                  = "test"
  secret_key                  = "test"
  region                      = "us-east-1"
  s3_use_path_style           = true
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    s3             = "http://localhost:4566"
    secretsmanager = "http://localhost:4566"
    ecs            = "http://localhost:4566"
    ec2            = "http://localhost:4566"
    iam            = "http://localhost:4566"
  }
}

# S3 Bucket for Compliance Reports
resource "aws_s3_bucket" "compliance_reports" {
  bucket = "openguard-compliance-reports"
  force_destroy = true
}

# Use Data source to avoid conflict if already exists
data "aws_secretsmanager_secret" "mtls_ca_cert" {
  name = "openguard/mtls/ca-cert"
}

resource "aws_secretsmanager_secret_version" "mtls_ca_cert_v1" {
  secret_id     = data.aws_secretsmanager_secret.mtls_ca_cert.id
  secret_string = "placeholder-ca-cert"
}
