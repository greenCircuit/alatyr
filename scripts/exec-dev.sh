#!/bin/bash
# exec into sandbox docker/podman container where have node installed
podman exec -it -e KUBECONFIG=/etc/rancher/k3s/k3s.yaml sandbox /bin/bash