module "vpc" {
  source = "./modules/vpc"
  cidr   = var.vpc_cidr
}

module "s3" {
  source = "./modules/s3"
  bucket_name = "compliance-reports"
}

module "secrets" {
  source = "./modules/secrets"
  secret_name = "openguard/jwt-keys"
}

module "eks" {
  count        = var.deployment_strategy == "eks" ? 1 : 0
  source       = "./modules/eks"
  cluster_name = "openguard-cluster"
  subnet_ids   = module.vpc.subnet_ids
}

module "ecs" {
  count        = var.deployment_strategy == "ecs" ? 1 : 0
  source       = "./modules/ecs"
  cluster_name = "openguard-cluster"
  vpc_id       = module.vpc.vpc_id
  subnet_ids   = module.vpc.subnet_ids
}

/*
resource "kubernetes_namespace" "openguard" {
  count = var.deployment_strategy == "eks" ? 1 : 0
  metadata {
    name = "openguard"
  }
}
*/

# --- ECS Services ---

module "iam_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "iam"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/iam:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "USE_AWS_SECRETS_MANAGER", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "policy_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "policy"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/policy:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "USE_AWS_SECRETS_MANAGER", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "POLICY_DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "audit_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "audit"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/audit:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "USE_AWS_SECRETS_MANAGER", value = "true" },
    { name = "AWS_REGION", value = var.aws_region },
    { name = "MONGODB_URI", value = "mongodb://mongo:27017" }
  ]
}

module "compliance_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "compliance"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/compliance:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "S3_ENDPOINT", value = "http://localstack:4566" },
    { name = "S3_BUCKET", value = "compliance-reports" },
    { name = "CLICKHOUSE_ADDR", value = "clickhouse:9000" }
  ]
}

module "alerting_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "alerting"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/alerting:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "MONGO_URI", value = "mongodb://mongo:27017" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "dlp_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "dlp"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/dlp:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "connector_registry_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "connector-registry"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/connector-registry:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" }
  ]
}

module "control_plane_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "control-plane"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/control-plane:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "threat_service_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "threat"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/threat:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "MONGO_URI", value = "mongodb://mongo:27017" },
    { name = "REDIS_ADDR", value = "redis:6379" }
  ]
}

module "webhook_delivery_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "webhook-delivery"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/webhook-delivery:latest"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
}

module "task_backend_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_connected_app) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "task-backend"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/task-backend:latest"
  container_port          = 3005
  subnet_ids              = module.vpc.subnet_ids
  environment = [
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" }
  ]
}

module "gateway_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "gateway"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "nginx:alpine"
  container_port          = 8080
  subnet_ids              = module.vpc.subnet_ids
}

module "openguard_ui_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_openguard) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "openguard-ui"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/web:latest"
  container_port          = 80
  subnet_ids              = module.vpc.subnet_ids
}

module "task_ui_ecs" {
  count                   = (var.deployment_strategy == "ecs" && var.deploy_connected_app) ? 1 : 0
  source                  = "./modules/ecs_service"
  service_name            = "task-ui"
  cluster_id              = module.ecs[0].cluster_id
  task_execution_role_arn = module.ecs[0].task_execution_role_arn
  container_image         = "openguard/task-app:latest"
  container_port          = 3000
  subnet_ids              = module.vpc.subnet_ids
}

/*
# --- EKS Services ---

module "iam_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "iam"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/iam:latest"
  container_port  = 8080
  env = [
    { name = "USE_AWS_SECRETS_MANAGER", value = "true" },
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "policy_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "policy"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/policy:latest"
  container_port  = 8080
  env = [
    { name = "POLICY_DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "audit_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "audit"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/audit:latest"
  container_port  = 8080
  env = [
    { name = "MONGODB_URI", value = "mongodb://mongo:27017" }
  ]
}

module "compliance_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "compliance"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/compliance:latest"
  container_port  = 8080
  env = [
    { name = "S3_ENDPOINT", value = "http://localstack:4566" },
    { name = "S3_BUCKET", value = "compliance-reports" }
  ]
}

module "gateway_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "gateway"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "nginx:alpine"
  container_port  = 8080
}

module "alerting_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "alerting"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/alerting:latest"
  container_port  = 8080
  env = [
    { name = "MONGO_URI", value = "mongodb://mongo:27017" },
    { name = "REDIS_URL", value = "redis://redis:6379/0" }
  ]
}

module "dlp_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "dlp"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/dlp:latest"
  container_port  = 8080
  env = [
    { name = "DATABASE_URL", value = "postgres://openguard:openguard@postgres:5432/openguard?sslmode=disable" }
  ]
}

module "connector_registry_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "connector-registry"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/connector-registry:latest"
  container_port  = 8080
}

module "control_plane_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "control-plane"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/control-plane:latest"
  container_port  = 8080
}

module "threat_service_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "threat"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/threat:latest"
  container_port  = 8080
}

module "webhook_delivery_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "webhook-delivery"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/webhook-delivery:latest"
  container_port  = 8080
}

module "task_backend_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_connected_app) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "task-backend"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/task-backend:latest"
  container_port  = 3005
}

module "openguard_ui_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_openguard) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "openguard-ui"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/web:latest"
  container_port  = 80
}

module "task_ui_eks" {
  count           = (var.deployment_strategy == "eks" && var.deploy_connected_app) ? 1 : 0
  source          = "./modules/k8s_service"
  service_name    = "task-ui"
  namespace       = length(kubernetes_namespace.openguard) > 0 ? kubernetes_namespace.openguard[0].metadata[0].name : ""
  container_image = "openguard/task-app:latest"
  container_port  = 3000
}
*/
