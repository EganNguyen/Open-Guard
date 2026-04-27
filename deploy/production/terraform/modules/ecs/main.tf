variable "environment" { type = string }
variable "vpc_id" { type = string }
variable "private_subnets" { type = list(string) }
variable "public_subnets" { type = list(string) }
variable "execution_role_arn" { type = string }
variable "discovery_namespace_id" { type = string }

# 1. ECS Cluster
resource "aws_ecs_cluster" "main" {
  name = "openguard-${var.environment}-cluster"
}

# 2. IAM Task Role (Permissions for the app itself)
resource "aws_iam_role" "ecs_task_role" {
  name = "openguard-${var.environment}-ecs-task-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

# 3. Microservice Service Template (Example: IAM)
resource "aws_ecs_task_definition" "iam" {
  family                   = "iam"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = var.execution_role_arn
  task_role_arn            = aws_iam_role.ecs_task_role.arn

  container_definitions = jsonencode([
    {
      name      = "iam"
      image     = "openguard/iam:latest" # Managed by CI/CD
      essential = true
      portMappings = [{ containerPort = 8080, hostPort = 8080 }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = "/ecs/openguard-${var.environment}"
          "awslogs-region"        = "us-east-1"
          "awslogs-stream-prefix" = "iam"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "iam" {
  name            = "iam"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.iam.arn
  desired_count   = 2
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.private_subnets
    security_groups = [aws_security_group.ecs_service.id]
  }

  service_registries {
    registry_arn = aws_service_discovery_service.iam.arn
  }
}

resource "aws_service_discovery_service" "iam" {
  name = "iam"
  dns_config {
    namespace_id = var.discovery_namespace_id
    dns_records {
      ttl  = 10
      type = "A"
    }
  }
}

resource "aws_security_group" "ecs_service" {
  name   = "openguard-${var.environment}-ecs-service-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["10.0.0.0/16"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
