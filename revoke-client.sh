#!/bin/bash

# Script to revoke an OpenVPN client
# Usage: ./revoke-client.sh CLIENTNAME

if [ -z "$1" ]; then
    echo "Usage: $0 CLIENTNAME"
    echo "Example: $0 client1"
    exit 1
fi

CLIENT_NAME=$1

echo "Revoking client: $CLIENT_NAME"

# Revoke the client certificate
docker compose run --rm openvpn easyrsa revoke "$CLIENT_NAME"

if [ $? -eq 0 ]; then
    echo "Generating updated CRL..."
    docker compose run --rm openvpn easyrsa gen-crl

    # Update CRL in the openvpn directory
    docker compose run --rm openvpn ovpn_otp_user

    echo ""
    echo "✓ Client '$CLIENT_NAME' revoked successfully!"
    echo "✓ Certificate Revocation List (CRL) updated"
    echo ""
    echo "Restarting OpenVPN server to apply changes..."
    docker compose restart openvpn

    echo "Client '$CLIENT_NAME' can no longer connect."
else
    echo "Error revoking client certificate"
    exit 1
fi
