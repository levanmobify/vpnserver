#!/bin/bash

# Script to add OpenVPN clients
# Usage: ./add-client.sh CLIENTNAME

if [ -z "$1" ]; then
    echo "Usage: $0 CLIENTNAME"
    echo "Example: $0 client1"
    exit 1
fi

CLIENT_NAME=$1

echo "Creating client certificate for: $CLIENT_NAME"

# Generate client certificate without password
docker compose run --rm openvpn easyrsa build-client-full "$CLIENT_NAME" nopass

if [ $? -eq 0 ]; then
    echo "Certificate created successfully!"
    echo "Generating .ovpn configuration file..."

    # Create clients directory if it doesn't exist
    mkdir -p clients

    # Generate client configuration file
    docker compose run --rm openvpn ovpn_getclient "$CLIENT_NAME" > "clients/$CLIENT_NAME.ovpn"

    if [ $? -eq 0 ]; then
        echo ""
        echo "✓ Client '$CLIENT_NAME' created successfully!"
        echo "✓ Configuration file saved to: clients/$CLIENT_NAME.ovpn"
        echo ""
        echo "You can now distribute clients/$CLIENT_NAME.ovpn to your users."
        echo "Multiple devices can use the same configuration file (duplicate-cn is enabled)."
    else
        echo "Error generating .ovpn file"
        exit 1
    fi
else
    echo "Error creating client certificate"
    exit 1
fi
