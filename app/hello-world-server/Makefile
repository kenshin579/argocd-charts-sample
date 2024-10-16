REGISTRY 	:= kenshin579
APP    		:= hello-world-server
IMAGE       := $(REGISTRY)/$(APP):v0.3

.PHONY: docker-build
docker-build:
	@docker build -t $(IMAGE) -f Dockerfile .

.PHONY: docker-push
docker-push: docker-build
	@docker push $(IMAGE)
