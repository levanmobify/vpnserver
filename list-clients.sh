#!/bin/bash

# Script to list all OpenVPN clients

echo "Listing all OpenVPN clients..."
echo ""

docker compose run --rm openvpn ovpn_listclients
