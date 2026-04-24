variable "cluster_name" { type = string }
variable "subnet_ids" { type = list(string) }

resource "aws_eks_cluster" "this" {
  name     = var.cluster_name
  role_arn = "arn:aws:iam::000000000000:role/eks-role"

  vpc_config {
    subnet_ids = var.subnet_ids
  }
}

output "cluster_endpoint" {
  value = aws_eks_cluster.this.endpoint
}

output "cluster_certificate_authority" {
  value = aws_eks_cluster.this.certificate_authority[0].data
}
