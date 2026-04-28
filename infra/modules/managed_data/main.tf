variable "environment" { type = string }
variable "vpc_id" { type = string }
variable "private_subnets" { type = list(string) }
variable "is_localstack" {
  type    = bool
  default = false
}

# 1. Aurora PostgreSQL Serverless v2 (Main State Store)
resource "aws_rds_cluster" "postgresql" {
  count                   = var.is_localstack ? 0 : 1
  cluster_identifier      = "openguard-${var.environment}-db"
  engine                  = "aurora-postgresql"
  engine_mode             = "provisioned"
  engine_version          = "15.4"
  database_name           = "openguard"
  master_username         = "openguardadmin"
  manage_master_user_password = true
  
  vpc_security_group_ids = [aws_security_group.db.id]
  db_subnet_group_name   = aws_db_subnet_group.main.name
  
  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 16.0
  }
}

resource "aws_rds_cluster_instance" "postgresql" {
  count              = var.is_localstack ? 0 : 1
  cluster_identifier = aws_rds_cluster.postgresql[0].id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.postgresql[0].engine
  engine_version     = aws_rds_cluster.postgresql[0].engine_version
}

# 2. ElastiCache for Redis Serverless (Policy Cache)
resource "aws_elasticache_serverless_cache" "redis" {
  count  = var.is_localstack ? 0 : 1
  engine = "redis"
  name   = "openguard-${var.environment}-redis"
  
  cache_usage_limits {
    data_storage {
      maximum = 10
      unit    = "GB"
    }
  }
  
  subnet_ids         = var.private_subnets
  security_group_ids = [aws_security_group.db.id]
}

# 3. MSK Serverless (Kafka Event Bus)
resource "aws_msk_serverless_cluster" "kafka" {
  count        = var.is_localstack ? 0 : 1
  cluster_name = "openguard-${var.environment}-kafka"

  vpc_config {
    subnet_ids         = var.private_subnets
    security_group_ids = [aws_security_group.db.id]
  }

  client_authentication {
    sasl {
      iam { enabled = true }
    }
  }
}

# Security Groups and Subnet Groups
resource "aws_db_subnet_group" "main" {
  name       = "openguard-${var.environment}-db-subnets"
  subnet_ids = var.private_subnets
}

resource "aws_security_group" "db" {
  name   = "openguard-${var.environment}-db-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["10.0.0.0/16"] # Internal only
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

output "postgresql_endpoint" { value = var.is_localstack ? "localhost" : aws_rds_cluster.postgresql[0].endpoint }
output "redis_endpoint" { value = var.is_localstack ? "localhost" : aws_elasticache_serverless_cache.redis[0].endpoint[0].address }
output "kafka_bootstrap_brokers" { value = var.is_localstack ? "localhost:9092" : aws_msk_serverless_cluster.kafka[0].arn }
 # MSK Serverless uses ARN for bootstrap in some SDKs or custom discovery
