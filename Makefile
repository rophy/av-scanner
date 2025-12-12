.PHONY: help build deploy clean test

IMAGE_NAME ?= av-scanner
IMAGE_TAG ?= latest
IMAGE_FILE ?= av-scanner-image.tar
VM_NAME ?= av-scanner

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Build:"
	@echo "  build      Build Docker image containing Go binary"
	@echo "  clean      Remove local image and tarball"
	@echo ""
	@echo "Deployment:"
	@echo "  deploy     Build and deploy to VM via Ansible"
	@echo ""
	@echo "Testing:"
	@echo "  test       Upload EICAR test file to VM and verify detection"

# Build the Docker image (contains Go binary)
build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Save image to tarball for transfer
save: build
	docker save $(IMAGE_NAME):$(IMAGE_TAG) -o $(IMAGE_FILE)

# Deploy to VM using Ansible (extracts binary from image)
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

# EICAR test string (base64 encoded to avoid shell escaping issues)
EICAR_B64 := WDVPIVAlQEFQWzRcUFpYNTQoUF4pN0NDKTd9JEVJQ0FSLVNUQU5EQVJELUFOVElWSVJVUy1URVNULUZJTEUhJEgrSCo=

# Test EICAR detection on VM
test:
	$(eval AV_SCANNER_IP := $(shell multipass info $(VM_NAME) --format csv | tail -1 | cut -d, -f3))
	@echo "Testing EICAR detection on $(AV_SCANNER_IP)..."
	@echo "$(EICAR_B64)" | base64 -d > /tmp/eicar-test.com
	@curl -s -X POST -F "file=@/tmp/eicar-test.com" "http://$(AV_SCANNER_IP):3000/api/v1/scan" | jq .
	@rm -f /tmp/eicar-test.com
