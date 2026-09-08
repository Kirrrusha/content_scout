#!/usr/bin/env bash
set -euo pipefail

release_dir="${CONTENT_SCOUT_RELEASE_DIR:-/opt/content_scout}"

sudo apt-get update
sudo apt-get install -y --no-install-recommends ca-certificates curl gnupg jq ufw

sudo install -m 0755 -d /etc/apt/keyrings
if [[ ! -f /etc/apt/keyrings/docker.gpg ]]; then
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg
fi

if [[ ! -f /etc/apt/sources.list.d/docker.list ]]; then
  . /etc/os-release
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
    | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
fi

sudo apt-get update
sudo apt-get install -y --no-install-recommends docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker scout

if ! sudo swapon --show=NAME | grep -qx /swapfile; then
  sudo fallocate -l 2G /swapfile
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile
  sudo swapon /swapfile
fi

if ! grep -q '^/swapfile ' /etc/fstab; then
  echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi

sudo mkdir -p "$release_dir"
sudo chown -R scout:scout "$release_dir"

sudo ufw allow OpenSSH
# Containers reach the SOCKS tunnel through the docker network gateway; the
# subnet is pinned in deployments/compose/docker-compose.prod.yml.
sudo ufw allow from 172.18.0.0/16 to 172.18.0.1 port 1080 proto tcp comment 'socks tunnel for containers'
sudo ufw --force enable
