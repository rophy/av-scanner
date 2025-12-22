REGISTRY ?= localhost:5000
IMAGE := $(REGISTRY)/kube-auth-proxy
TAG ?= latest

.PHONY: build push deploy clean

build:
	podman build -t $(IMAGE):$(TAG) .

push: build
	podman push --tls-verify=false $(IMAGE):$(TAG)

deploy:
	kubectl apply -k k8s/

undeploy:
	kubectl delete -k k8s/ --ignore-not-found

clean:
	podman rmi $(IMAGE):$(TAG) || true

all: push deploy
