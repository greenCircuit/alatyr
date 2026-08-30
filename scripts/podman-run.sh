#!/bin/bash
# run sandbox dev environment
podman run -d --name sandbox -it \
    --network=host \
    -v ./:/app \
    -v ~/.gitconfig:/root/.gitconfig \
    -v /home/$USER/.local/bin/claude:/usr/local/bin/claude \
    -v /home/$USER/.claude:/root/.claude \
    -v /home/$USER/.vscode/extensions:/root/.vscode-server/extensions \
    --memory 4g \
    -e KUBECONFIG=/root/k3s.yaml \
    --replace localhost/sandbox:latest /bin/bash

