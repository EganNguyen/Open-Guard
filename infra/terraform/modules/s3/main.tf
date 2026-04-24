variable "bucket_name" { type = string }

resource "aws_s3_bucket" "this" {
  bucket        = var.bucket_name
  force_destroy = true
}

output "bucket_arn" {
  value = aws_s3_bucket.this.arn
}
