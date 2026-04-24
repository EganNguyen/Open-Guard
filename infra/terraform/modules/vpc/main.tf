variable "cidr" { type = string }

resource "aws_vpc" "main" {
  cidr_block = var.cidr
}

resource "aws_subnet" "a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.cidr, 8, 1)
  availability_zone = "us-east-1a"
}

resource "aws_subnet" "b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrsubnet(var.cidr, 8, 2)
  availability_zone = "us-east-1b"
}

output "subnet_ids" {
  value = [aws_subnet.a.id, aws_subnet.b.id]
}

output "vpc_id" {
  value = aws_vpc.main.id
}
