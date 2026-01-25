#!/bin/bash

# Lists all OpenVPN clients

if [ ! -f openvpn-data/openvpn.conf ]; then
    echo "ERROR: OpenVPN not initialized"
    exit 1
fi

echo "=== OpenVPN Clients ==="
docker compose run --rm openvpn ovpn_listclients
