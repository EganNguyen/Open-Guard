variable "environment" { type = string }
variable "vpc_id" { type = string }
variable "private_subnets" { type = list(string) }

# 1. Cloud-Init for MongoDB
data "cloudinit_config" "mongodb" {
  gzip          = false
  base64_encode = true

  part {
    content_type = "text/cloud-config"
    content      = <<-EOF
      #cloud-config
      runcmd:
        - apt-get update
        - apt-get install -y gnupg wget apt-transport-https
        - wget -qO - https://www.mongodb.org/static/pgp/server-6.0.asc | apt-key add -
        - echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/ubuntu jammy/mongodb-org/6.0 multiverse" | tee /etc/apt/sources.list.d/mongodb-org-6.0.list
        - apt-get update
        - apt-get install -y mongodb-org
        - systemctl enable mongod
        - systemctl start mongod
        - sed -i "s/127.0.0.1/0.0.0.0/" /etc/mongod.conf
        - systemctl restart mongod
    EOF
  }
}

# 2. Cloud-Init for ClickHouse
data "cloudinit_config" "clickhouse" {
  gzip          = false
  base64_encode = true

  part {
    content_type = "text/cloud-config"
    content      = <<-EOF
      #cloud-config
      runcmd:
        - apt-get update
        - apt-get install -y apt-transport-https ca-certificates dirmngr
        - apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv 8919F6BD2B48D754
        - echo "deb https://packages.clickhouse.com/deb stable main" | tee /etc/apt/sources.list.d/clickhouse.list
        - apt-get update
        - DEBIAN_FRONTEND=noninteractive apt-get install -y clickhouse-server clickhouse-client
        - systemctl enable clickhouse-server
        - systemctl start clickhouse-server
        - sed -i "s/<!-- <listen_host>::<\/listen_host> -->/<listen_host>0.0.0.0<\/listen_host>/" /etc/clickhouse-server/config.xml
        - systemctl restart clickhouse-server
    EOF
  }
}

# 3. Auto-Scaling Group for Mongo
resource "aws_launch_template" "mongodb" {
  name_prefix   = "openguard-${var.environment}-mongodb"
  image_id      = "ami-053b0d53c279acc90" # Ubuntu 22.04 LTS
  instance_type = "t3.medium"
  user_data     = data.cloudinit_config.mongodb.rendered

  vpc_security_group_ids = [aws_security_group.standalone.id]
  
  block_device_mappings {
    device_name = "/dev/sda1"
    ebs {
      volume_size = 50
      encrypted   = true
    }
  }
}

resource "aws_autoscaling_group" "mongodb" {
  desired_capacity    = 1
  max_size            = 1
  min_size            = 1
  vpc_zone_identifier = var.private_subnets

  launch_template {
    id      = aws_launch_template.mongodb.id
    version = "$Latest"
  }
}

# Security Group
resource "aws_security_group" "standalone" {
  name   = "openguard-${var.environment}-standalone-sg"
  vpc_id = var.vpc_id

  ingress {
    from_port   = 27017 # Mongo
    to_port     = 27017
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }

  ingress {
    from_port   = 8123 # ClickHouse HTTP
    to_port     = 9000 # ClickHouse Native
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
