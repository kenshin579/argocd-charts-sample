NAMESPACE := argocd-test

# 명령어를 자동으로 감지하는 함수
define KUBEVPN_CMD
$(shell if which kubevpn >/dev/null 2>&1; then echo "kubevpn"; else echo "kubectl kubevpn"; fi)
endef

.PHONY: all
all: tf-init tf-infra

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

.PHONY: clean-kubevpn
clean-kubevpn:
	@echo "Cleaning up kubevpn..."
	@sudo $(KUBEVPN_CMD) disconnect -n $(NAMESPACE) --all
	@echo "\nkubevpn cleanup completed."

.PHONY: setup-kubevpn
setup-kubevpn:
	@echo "Setting up kubevpn..."
	@sudo $(KUBEVPN_CMD) connect -n $(NAMESPACE) --netstack gvisor
	@echo "\nkubevpn setup completed."

.PHONY: tf-clean
tf-clean: clean-kubevpn
	@echo "Force cleaning Terraform state..."
	@rm -rf .terraform/
	@rm -f .terraform.lock.hcl
	@rm -f terraform.tfstate
	@rm -f terraform.tfstate.backup
	@echo "Terraform cleanup completed."
