variable "argotest_namespace" {
  type    = string
  default = "argocd-test"
}

variable "kubeconfig" {
  type    = string
  default = "~/.kube/config"
}

