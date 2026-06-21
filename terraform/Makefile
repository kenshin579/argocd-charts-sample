NAMESPACE := argocd-test
CLUSTER_NAME := argo-cluster

.PHONY: kind-create
kind-create:
	@echo "Creating kind cluster ($(CLUSTER_NAME)) if not exists..."
	@kind get clusters | grep -q '^$(CLUSTER_NAME)$$' || kind create cluster --config kind-config.yaml

.PHONY: kind-delete
kind-delete:
	@echo "Deleting kind cluster ($(CLUSTER_NAME))..."
	@kind delete cluster --name $(CLUSTER_NAME)

.PHONY: all
all: kind-create tf-init tf-infra

.PHONY: tf-init
tf-init:
	@echo "Initializing Terraform for Docker Desktop cluster..."
	@terraform init -upgrade

.PHONY: tf-infra
tf-infra: tf-init
	@echo "Setting up infrastructure on Docker Desktop cluster..."
	@terraform plan
	@terraform apply -auto-approve

.PHONY: tf-validate
tf-validate:
	@echo "Validating Terraform configuration..."
	@terraform validate

.PHONY: tf-destroy
tf-destroy:
	@echo "Destroying Terraform resources..."
	@terraform plan -destroy
	@terraform destroy -auto-approve

.PHONY: view-argocd-password
view-argocd-password:
	@kubectl -n argocd get secret argocd-secret -o jsonpath="{.data.admin\.password}" | base64 -d; echo

.PHONY: argocd-port-forward
argocd-port-forward:
	@echo "ArgoCD UI → https://localhost:8080 (Ctrl-C로 종료)"
	@kubectl port-forward svc/argocd-server -n argocd 8080:443

.PHONY: tf-clean
tf-clean:
	@echo "Force cleaning Terraform state..."
	@rm -rf .terraform/
	@rm -f .terraform.lock.hcl
	@rm -f terraform.tfstate
	@rm -f terraform.tfstate.backup
	@echo "Terraform cleanup completed."
