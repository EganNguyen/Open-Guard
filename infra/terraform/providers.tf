provider "aws" {
  region                      = var.aws_region
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    s3             = var.localstack_endpoint
    secretsmanager = var.localstack_endpoint
    eks            = var.localstack_endpoint
    ecs            = var.localstack_endpoint
    iam            = var.localstack_endpoint
    ec2            = var.localstack_endpoint
  }
}

/*
provider "kubernetes" {
  host                   = var.deployment_strategy == "eks" && length(module.eks) > 0 ? module.eks[0].cluster_endpoint : "https://localhost"
  cluster_ca_certificate = var.deployment_strategy == "eks" && length(module.eks) > 0 ? base64decode(module.eks[0].cluster_certificate_authority) : ""
  token                  = "mock-token"
}
*/
