.PHONY: help build deploy clean dev dev-down

IMAGE_NAME ?= av-scanner
IMAGE_TAG ?= latest
IMAGE_FILE ?= av-scanner-image.tar
VM_NAME ?= av-scanner

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Development:"
	@echo "  dev        Start dev container and shell into it"
	@echo "  dev-down   Stop dev container"
	@echo ""
	@echo "Production:"
	@echo "  build      Build Docker image locally"
	@echo "  deploy     Build and deploy to VM via Ansible"
	@echo "  clean      Remove local image and tarball"

# Build the Docker image locally
build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Save image to tarball for transfer
save: build
	docker save $(IMAGE_NAME):$(IMAGE_TAG) -o $(IMAGE_FILE)

# Deploy to VM using Ansible (transfers and loads image)
deploy: save
	$(eval AV_SCANNER_IP := $(shell multipass info $(VM_NAME) --format csv | tail -1 | cut -d, -f3))
	. venv/bin/activate && cd ansible && AV_SCANNER_IP=$(AV_SCANNER_IP) ansible-playbook deploy.yaml \
		-e image_file=$(CURDIR)/$(IMAGE_FILE) \
		-e image_name=$(IMAGE_NAME) \
		-e image_tag=$(IMAGE_TAG)

# Clean up build artifacts
clean:
	rm -f $(IMAGE_FILE)
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true

# Start dev container and shell into it
dev:
	@echo "UID=$$(id -u)" > .env
	@echo "GID=$$(id -g)" >> .env
	docker compose up -d --build
	docker compose exec av-scanner sh

# Stop dev container
dev-down:
	docker compose down
