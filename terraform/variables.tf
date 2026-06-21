variable "argotest_namespace" {
  type    = string
  default = "argocd-test"
}

variable "kubeconfig" {
  type    = string
  default = "~/.kube/config"
}

variable "kube_context" {
  type    = string
  default = "kind-argo-cluster"
}

