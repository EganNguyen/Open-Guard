module "networking" {
  source = "../../modules/networking"

  vpc_cidr      = var.vpc_cidr
  environment   = var.environment
  domain_name   = var.domain_name
  is_localstack = var.is_localstack
}

module "security" {
  source = "../../modules/security"

  environment   = var.environment
  domain_name   = var.domain_name
  is_localstack = var.is_localstack
}

module "managed_data" {
  source = "../../modules/managed_data"

  environment     = var.environment
  vpc_id          = module.networking.vpc_id
  private_subnets = module.networking.private_subnets
  is_localstack   = var.is_localstack
}

module "standalone_data" {
  source = "../../modules/standalone_data"

  environment     = var.environment
  vpc_id          = module.networking.vpc_id
  private_subnets = module.networking.private_subnets
  is_localstack   = var.is_localstack
}

module "waf" {
  source = "../../modules/waf"

  environment         = var.environment
  vpc_id              = module.networking.vpc_id
  public_subnets      = module.networking.public_subnets
  acm_certificate_arn = module.security.acm_certificate_arn
  is_localstack       = var.is_localstack
}

module "ecs" {
  source = "../../modules/ecs"

  environment            = var.environment
  vpc_id                 = module.networking.vpc_id
  private_subnets        = module.networking.private_subnets
  public_subnets         = module.networking.public_subnets
  execution_role_arn     = module.security.ecs_execution_role_arn
  discovery_namespace_id = module.networking.service_discovery_namespace_id
  image_tag              = var.image_tag
  is_localstack          = var.is_localstack
}

