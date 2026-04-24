variable "service_name" { type = string }
variable "cluster_id" { type = string }
variable "task_execution_role_arn" { type = string }
variable "container_image" { type = string }
variable "container_port" { type = number }
variable "subnet_ids" { type = list(string) }
variable "environment" {
  type = list(object({
    name  = string
    value = string
  }))
  default = []
}

resource "aws_ecs_task_definition" "this" {
  family                   = var.service_name
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = var.task_execution_role_arn

  container_definitions = jsonencode([
    {
      name      = var.service_name
      image     = var.container_image
      essential = true
      portMappings = [
        {
          containerPort = var.container_port
          hostPort      = var.container_port
        }
      ]
      environment = var.environment
    }
  ])
}

resource "aws_ecs_service" "this" {
  name            = var.service_name
  cluster         = var.cluster_id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.subnet_ids
    assign_public_ip = true
  }
}
