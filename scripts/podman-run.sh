#!/bin/bash
# run sandbox dev environment
podman run -d --name sandbox -it \
    --network=host \
    -v ./:/app \
    -v ~/.gitconfig:/root/.gitconfig \
    -v /etc/rancher/k3s/k3s.yaml:/etc/rancher/k3s/k3s.yaml:ro \
    --replace localhost/sandbox:latest /bin/bash
