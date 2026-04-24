variable "service_name" { type = string }
variable "namespace" { type = string }
variable "container_image" { type = string }
variable "container_port" { type = number }
variable "replicas" {
  type    = number
  default = 1
}
variable "env" {
  type = list(object({
    name  = string
    value = string
  }))
  default = []
}

resource "kubernetes_deployment" "this" {
  metadata {
    name      = var.service_name
    namespace = var.namespace
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = {
        app = var.service_name
      }
    }

    template {
      metadata {
        labels = {
          app = var.service_name
        }
      }

      spec {
        container {
          name  = var.service_name
          image = var.container_image

          port {
            container_port = var.container_port
          }

          dynamic "env" {
            for_each = var.env
            content {
              name  = env.value.name
              value = env.value.value
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "this" {
  metadata {
    name      = var.service_name
    namespace = var.namespace
  }

  spec {
    selector = {
      app = var.service_name
    }

    port {
      port        = var.container_port
      target_port = var.container_port
    }

    type = "ClusterIP"
  }
}
