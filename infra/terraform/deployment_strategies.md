# Deployment Strategies: ECS Fargate vs EKS

OpenGuard supports two primary deployment strategies for AWS: **Amazon ECS Fargate** and **Amazon EKS (Kubernetes)**. 

## Overview
OpenGuard supports two primary deployment strategies for AWS: **Amazon ECS Fargate** and **Amazon EKS (Kubernetes)**. Both strategies provide full automation for:
- **10+ Core Backend Services**: IAM, Policy, Audit, etc.
- **OpenGuard Dashboard (Web UI)**: The administrative interface.
- **Connected App**: The example Task Management application (Backend & Frontend).
- **Infrastructure Dependencies**: S3, Secrets Manager, and Networking.

## How to Choose

You can switch between strategies by setting the `deployment_strategy` variable in Terraform.

### 1. Using ECS Fargate (Cost-Effective)
To use ECS Fargate, set the strategy to `ecs`. This is recommended for development, testing, and small to medium production workloads to save on the $70+/month EKS control plane fee.

```bash
terraform apply -var="deployment_strategy=ecs" -refresh=false
```

### 2. Using EKS (Standard)
To use EKS, set the strategy to `eks` (default). This is recommended for complex microservices architectures or teams already standardized on Kubernetes.

```bash
terraform apply -var="deployment_strategy=eks" -refresh=false
```

### 3. Selective Deployment
You can now deploy OpenGuard core services and the example Connected App separately using the following flags:

**Deploy only OpenGuard Core (Dashboard + Services):**
```bash
terraform apply -var="deployment_strategy=ecs" -var="deploy_connected_app=false"
```

**Deploy only the Connected App (Task Backend + Frontend):**
```bash
terraform apply -var="deployment_strategy=ecs" -var="deploy_openguard=false"
```

Both are enabled by default (`true`).

### Shared Resources
Regardless of the strategy, the following resources are provisioned:
- **VPC & Subnets**: Base networking.
- **S3 Bucket**: For compliance reports.
- **Secrets Manager**: For JWT keys and other secrets.

### ECS Specifics
When `deployment_strategy = "ecs"`:
- An **ECS Cluster** is created.
- **ECS Task Definitions** and **Services** are provisioned (using the `ecs_service` module).
- **IAM Roles** for task execution are automatically created.

### EKS Specifics
When `deployment_strategy = "eks"`:
- An **EKS Cluster** is created.
- A **Kubernetes Namespace** (`openguard`) is initialized.
- Kubernetes manifests in `infra/k8s/` can be applied to this cluster.

## Implementation Details

The implementation uses Terraform `count` to conditionally enable modules:

```hcl
module "eks" {
  count        = var.deployment_strategy == "eks" ? 1 : 0
  source       = "./modules/eks"
  # ...
}

module "ecs" {
  count        = var.deployment_strategy == "ecs" ? 1 : 0
  source       = "./modules/ecs"
  # ...
}
```
