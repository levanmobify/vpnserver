#!/bin/bash

# Usage: ./revoke-client.sh <client-name>

if [ -z "$1" ]; then
    echo "ERROR: Client name required"
    echo "Usage: ./revoke-client.sh <client-name>"
    exit 1
fi

CLIENT_NAME=$1

if [ ! -f openvpn-data/openvpn.conf ]; then
    echo "ERROR: OpenVPN not initialized"
    exit 1
fi

echo "Revoking client: $CLIENT_NAME"

# Revoke the client certificate
docker compose run --rm openvpn easyrsa revoke "$CLIENT_NAME"

# Regenerate CRL
docker compose run --rm openvpn easyrsa gen-crl

# Restart OpenVPN to apply changes
docker compose restart openvpn

echo "Client $CLIENT_NAME has been revoked"

# Remove local .ovpn file if it exists
if [ -f "clients/${CLIENT_NAME}.ovpn" ]; then
    rm "clients/${CLIENT_NAME}.ovpn"
    echo "Removed local configuration file"
fi
