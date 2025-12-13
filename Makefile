.PHONY: help build deploy clean test vm-init vm-start vm-stop setup-vm

IMAGE_NAME ?= av-scanner
IMAGE_TAG ?= latest
IMAGE_FILE ?= av-scanner-image.tar
VM_NAME ?= av-scanner
STATE_FILE ?= .vm-state

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "VM Management:"
	@echo "  vm-init    Create VM (auto-detects Multipass or QEMU)"
	@echo "  vm-start   Start existing VM"
	@echo "  vm-stop    Stop VM"
	@echo "  setup-vm   Install ClamAV on VM via Ansible"
	@echo ""
	@echo "Build & Deploy:"
	@echo "  build      Build Docker image containing Go binary"
	@echo "  deploy     Build and deploy to VM via Ansible"
	@echo "  clean      Remove local image and tarball"
	@echo ""
	@echo "Testing:"
	@echo "  test       Upload EICAR test file to VM and verify detection"
	@echo "  test-unit  Run unit tests"
	@echo "  test-mock  Test locally with mock driver"

# ============================================
# VM Management
# ============================================

# Create VM (auto-detects hypervisor)
vm-init:
	./scripts/vm-init.sh

# Start existing VM
vm-start:
	./scripts/vm-start.sh

# Stop VM
vm-stop:
	@if [ -f $(STATE_FILE) ]; then \
		. $(STATE_FILE); \
		if [ "$$HYPERVISOR" = "multipass" ]; then \
			multipass stop $$VM_NAME; \
		else \
			kill $$(cat $$QEMU_DIR/$$VM_NAME.pid) 2>/dev/null || true; \
		fi; \
		echo "VM stopped"; \
	else \
		echo "No VM state found"; \
	fi

# Install ClamAV on VM via Ansible
setup-vm:
	@if [ ! -f $(STATE_FILE) ]; then echo "Run 'make vm-init' first"; exit 1; fi
	@. $(STATE_FILE); \
	if [ -f venv/bin/activate ]; then . venv/bin/activate; fi; \
	cd ansible && \
	if [ "$$HYPERVISOR" = "multipass" ]; then \
		AV_SCANNER_IP=$$VM_IP ansible-playbook setup-vm.yaml -i inventory.yaml; \
	else \
		AV_SCANNER_IP=$$VM_IP AV_SCANNER_PORT=$$SSH_PORT AV_SCANNER_PASS=$$SSH_PASS \
		ansible-playbook setup-vm.yaml -i inventory.yaml; \
	fi

# ============================================
# Build
# ============================================

# Build the Docker image (contains Go binary)
build:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Save image to tarball for transfer
save: build
	docker save $(IMAGE_NAME):$(IMAGE_TAG) -o $(IMAGE_FILE)

# ============================================
# Deploy
# ============================================

# Deploy to VM using Ansible (extracts binary from image)
deploy: save
	@if [ ! -f $(STATE_FILE) ]; then echo "Run 'make vm-init' first"; exit 1; fi
	@. $(STATE_FILE); \
	if [ -f venv/bin/activate ]; then . venv/bin/activate; fi; \
	cd ansible && \
	if [ "$$HYPERVISOR" = "multipass" ]; then \
		AV_SCANNER_IP=$$VM_IP ansible-playbook deploy.yaml -i inventory.yaml \
			-e image_file=$(CURDIR)/$(IMAGE_FILE) \
			-e image_name=$(IMAGE_NAME) \
			-e image_tag=$(IMAGE_TAG); \
	else \
		AV_SCANNER_IP=$$VM_IP AV_SCANNER_PORT=$$SSH_PORT AV_SCANNER_PASS=$$SSH_PASS \
		ansible-playbook deploy.yaml -i inventory.yaml \
			-e image_file=$(CURDIR)/$(IMAGE_FILE) \
			-e image_name=$(IMAGE_NAME) \
			-e image_tag=$(IMAGE_TAG); \
	fi

# Clean up build artifacts
clean:
	rm -f $(IMAGE_FILE)
	docker rmi $(IMAGE_NAME):$(IMAGE_TAG) 2>/dev/null || true

# ============================================
# Testing
# ============================================

# EICAR test string (base64 encoded to avoid shell escaping issues)
EICAR_B64 := WDVPIVAlQEFQWzRcUFpYNTQoUF4pN0NDKTd9JEVJQ0FSLVNUQU5EQVJELUFOVElWSVJVUy1URVNULUZJTEUhJEgrSCo=

# Test EICAR detection on VM
test:
	@if [ ! -f $(STATE_FILE) ]; then echo "Run 'make vm-init' first"; exit 1; fi
	@. $(STATE_FILE); \
	if [ "$$HYPERVISOR" = "multipass" ]; then \
		API_URL="http://$$VM_IP:3000"; \
	else \
		API_URL="http://localhost:$$API_PORT"; \
	fi; \
	echo "Testing EICAR detection on $$API_URL..."; \
	echo "$(EICAR_B64)" | base64 -d > /tmp/eicar-test.com; \
	curl -s -X POST -F "file=@/tmp/eicar-test.com" "$$API_URL/api/v1/scan" | jq .; \
	rm -f /tmp/eicar-test.com

# Run unit tests
test-unit:
	go test -race ./...

# Test locally with mock driver
test-mock:
	@mkdir -p /tmp/av-scanner-test
	@echo "Starting server with mock driver on port 3333..."
	@AV_ENGINE=mock UPLOAD_DIR=/tmp/av-scanner-test PORT=3333 go run . & \
	PID=$$!; \
	sleep 1; \
	echo "Testing clean file..."; \
	echo "clean content" > /tmp/clean-test.txt; \
	curl -s -X POST -F "file=@/tmp/clean-test.txt" "http://localhost:3333/api/v1/scan" | jq .; \
	echo "Testing EICAR detection..."; \
	echo "$(EICAR_B64)" | base64 -d > /tmp/eicar-test.com; \
	curl -s -X POST -F "file=@/tmp/eicar-test.com" "http://localhost:3333/api/v1/scan" | jq .; \
	rm -f /tmp/clean-test.txt /tmp/eicar-test.com; \
	kill $$PID 2>/dev/null || true
