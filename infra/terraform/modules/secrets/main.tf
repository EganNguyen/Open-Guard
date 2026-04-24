variable "secret_name" { type = string }

resource "aws_secretsmanager_secret" "this" {
  name = var.secret_name
}

resource "aws_secretsmanager_secret_version" "this" {
  secret_id     = aws_secretsmanager_secret.this.id
  secret_string = jsonencode([
    {
      kid       = "k1"
      secret    = "super-secret-key-at-least-32-chars"
      algorithm = "HS256"
      status    = "active"
    }
  ])
}

output "secret_arn" {
  value = aws_secretsmanager_secret.this.arn
}
