variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "localstack_endpoint" {
  description = "LocalStack endpoint URL"
  type        = string
  default     = "http://localhost:4566"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "deployment_strategy" {
  description = "Deployment strategy: 'eks' or 'ecs'"
  type        = string
  default     = "eks"
}

variable "deploy_openguard" {
  description = "Whether to deploy OpenGuard core services and dashboard"
  type        = bool
  default     = true
}

variable "deploy_connected_app" {
  description = "Whether to deploy the connected example application (task-management-app)"
  type        = bool
  default     = true
}
