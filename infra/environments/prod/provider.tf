terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  # For initial bootstrap, this will be local. 
  # After bootstrap.sh runs, this will be migrated to S3/DynamoDB.
  # backend "s3" {}
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "OpenGuard"
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}
