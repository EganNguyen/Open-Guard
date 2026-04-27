variable "environment" { type = string }
variable "domain_name" { type = string }

# 1. IAM Execution Role for ECS (Pull images, write logs)
resource "aws_iam_role" "ecs_execution_role" {
  name = "openguard-${var.environment}-ecs-execution-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "ecs_execution_policy" {
  role       = aws_iam_role.ecs_execution_role.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# 2. KMS Key for Data Encryption (Audit Logs, S3)
resource "aws_kms_key" "main" {
  description             = "OpenGuard Master Encryption Key"
  deletion_window_in_days = 7
  enable_key_rotation     = true
}

resource "aws_kms_alias" "main" {
  name          = "alias/openguard-${var.environment}-key"
  target_key_id = aws_kms_key.main.key_id
}

# 3. ACM Certificate for openguard.com
# Note: Validation requires Route53 DNS entries (handled by default in Route53 module)
resource "aws_acm_certificate" "main" {
  domain_name       = var.domain_name
  subject_alternative_names = ["*.${var.domain_name}"]
  validation_method = "DNS"

  lifecycle { create_before_destroy = true }
}

output "ecs_execution_role_arn" { value = aws_iam_role.ecs_execution_role.arn }
output "kms_key_arn" { value = aws_kms_key.main.arn }
output "acm_certificate_arn" { value = aws_acm_certificate.main.arn }
