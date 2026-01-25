#!/bin/bash

# Usage: ./generate-client.sh <client-name>

if [ -z "$1" ]; then
    echo "ERROR: Client name required"
    echo "Usage: ./generate-client.sh <client-name>"
    exit 1
fi

CLIENT_NAME=$1

# Check if OpenVPN is initialized
if [ ! -f openvpn-data/openvpn.conf ]; then
    echo "ERROR: OpenVPN not initialized. Run start.sh first."
    exit 1
fi

# Check if client already exists
if docker compose run --rm openvpn ovpn_listclients | grep -q "^$CLIENT_NAME$"; then
    echo "WARNING: Client $CLIENT_NAME already exists"
    echo "Retrieving existing configuration..."
else
    echo "Generating new client: $CLIENT_NAME"
    # Generate client certificate without passphrase
    docker compose run --rm openvpn easyrsa build-client-full "$CLIENT_NAME" nopass
fi

# Get the .ovpn file content
echo "Retrieving .ovpn configuration..."
OVPN_CONTENT=$(docker compose run --rm openvpn ovpn_getclient "$CLIENT_NAME")

# Save to file
echo "$OVPN_CONTENT" > "clients/${CLIENT_NAME}.ovpn"

# Also output to stdout for Laravel to capture
echo "=== OVPN_CONFIG_START ==="
echo "$OVPN_CONTENT"
echo "=== OVPN_CONFIG_END ==="

echo "Client configuration saved to: clients/${CLIENT_NAME}.ovpn"
