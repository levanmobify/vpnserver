#!/bin/sh
# sudo modprobe af_key

# Create IPsec directories
mkdir -p etc/ipsec.d
mkdir -p etc/ppp
touch etc/ipsec.d/passwd
touch etc/ppp/chap-secrets
touch etc/ipsec.secrets

# Create OpenVPN directories
mkdir -p openvpn-data
mkdir -p clients

# Initialize OpenVPN if not already configured
if [ ! -f openvpn-data/openvpn.conf ]; then
    echo "Initializing OpenVPN..."

    # Fetch public IP dynamically
    PUBLIC_IP=$(curl -s http://ipv4.icanhazip.com)

    if [ -z "$PUBLIC_IP" ]; then
        echo "Failed to fetch public IP from icanhazip.com, trying alternative..."
        PUBLIC_IP=$(curl -s http://ip1.dynupdate.no-ip.com)
    fi

    if [ -z "$PUBLIC_IP" ]; then
        echo "ERROR: Could not fetch public IP address"
        exit 1
    fi

    echo "Detected public IP: $PUBLIC_IP"

    # Allow env overrides for non-interactive defaults
    OPENVPN_URL=${OPENVPN_URL:-udp://$PUBLIC_IP}
    OPENVPN_CA_CN=${OPENVPN_CA_CN:-"OpenVPN CA"}

    # Generate OpenVPN configuration
    docker compose run --rm openvpn ovpn_genconfig -u "$OPENVPN_URL"

    # Initialize PKI without passphrase (automated deployment)
    docker compose run --rm -e EASYRSA_BATCH=1 -e EASYRSA_REQ_CN="$OPENVPN_CA_CN" openvpn ovpn_initpki nopass

    echo "OpenVPN initialized successfully with IP: $PUBLIC_IP"
else
    echo "OpenVPN already configured, skipping initialization"
fi

docker compose up -d
