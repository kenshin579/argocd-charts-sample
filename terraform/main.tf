terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "2.37.1"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "3.0.1"
    }
  }
}

provider "kubernetes" {
  config_path    = var.kubeconfig
  config_context = "docker-desktop"
}

provider "helm" {
  kubernetes = {
    config_path    = var.kubeconfig
    config_context = "docker-desktop"
  }
}

module "infra" {
  source = "./modules/infra"
}
