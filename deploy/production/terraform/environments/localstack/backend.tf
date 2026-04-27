terraform {
  backend "s3" {
    bucket         = "openguard-terraform-state-localstack"
    key            = "openguard/localstack/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "openguard-terraform-locks-localstack"
    encrypt        = true
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    force_path_style            = true
    endpoints = { s3 = \"http://localhost:4566\", dynamodb = \"http://localhost:4566\" }
  }
}
