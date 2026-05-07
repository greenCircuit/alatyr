#!/bin/bash
# run sandbox dev environment
podman run -d --name sandbox -it \
    --network=host \
    -v ./:/app \
    -v ~/.gitconfig:/root/.gitconfig \
    -v /home/$USER/.local/bin/claude:/usr/local/bin/claude \
    -v /home/$USER/.claude:/root/.claude \
    -v /home/$USER/.vscode/extensions:/root/.vscode-server/extensions \
    -v ~/.kube/k3s.yaml:/root/k3s.yaml \
    -e KUBECONFIG=/root/k3s.yaml \
    --replace localhost/sandbox:latest /bin/bash

podman exec sandbox mkdir -p /home/$USER/Desktop/PycharmProjects
podman exec sandbox ln -sf /app /home/$USER/Desktop/PycharmProjects/network-policy-visualizer
