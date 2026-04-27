variable "vpc_cidr" { type = string }
variable "environment" { type = string }
variable "domain_name" { type = string }

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = { Name = "openguard-${var.environment}-vpc" }
}

# Public Subnets
resource "aws_subnet" "public" {
  count             = 3
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index)
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = { Name = "openguard-${var.environment}-public-${count.index}" }
}

# Private Subnets (App & Data)
resource "aws_subnet" "private" {
  count             = 3
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 8, count.index + 10)
  availability_zone = data.aws_availability_zones.available.names[count.index]

  tags = { Name = "openguard-${var.environment}-private-${count.index}" }
}

# Internet Gateway
resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "openguard-${var.environment}-igw" }
}

# NAT Gateways (One per AZ for HA)
resource "aws_eip" "nat" {
  count = 3
  tags  = { Name = "openguard-${var.environment}-nat-eip-${count.index}" }
}

resource "aws_nat_gateway" "main" {
  count         = 3
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id
  tags          = { Name = "openguard-${var.environment}-nat-${count.index}" }
}

# Routing
resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }
}

resource "aws_route_table_association" "public" {
  count          = 3
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  count  = 3
  vpc_id = aws_vpc.main.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.main[count.index].id
  }
}

resource "aws_route_table_association" "private" {
  count          = 3
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

# Route53 Public Zone
resource "aws_route53_zone" "public" {
  name = var.domain_name
}

# Route53 Private Zone for Service Discovery
resource "aws_service_discovery_private_dns_namespace" "internal" {
  name        = "openguard.local"
  description = "Private DNS for microservices"
  vpc         = aws_vpc.main.id
}

output "vpc_id" { value = aws_vpc.main.id }
output "public_subnets" { value = aws_subnet.public[*].id }
output "private_subnets" { value = aws_subnet.private[*].id }
output "hosted_zone_id" { value = aws_route53_zone.public.zone_id }
output "service_discovery_namespace_id" { value = aws_service_discovery_private_dns_namespace.internal.id }
