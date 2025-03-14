!/bin/sh
sudo modprobe af_key

mkdir -p etc/ipsec.d
mkdir -p etc/ppp
touch etc/ipsec.d/passwd
touch etc/ppp/chap-secrets
touch etc/ipsec.secrets

docker compose up -d