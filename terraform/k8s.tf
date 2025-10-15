resource "kubernetes_namespace" "argocd_test" {
  metadata {
    name = var.argotest_namespace
  }
}

resource "null_resource" "set_default_namespace" {
  depends_on = [kubernetes_namespace.argocd_test]

  provisioner "local-exec" {
    command = "kubectl config set-context docker-desktop --namespace=${var.argotest_namespace}"
  }
}
