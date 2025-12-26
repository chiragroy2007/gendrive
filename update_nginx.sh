#!/bin/bash
# Inject client_max_body_size 0 (unlimited) into Nginx config
# We look for the server block or server_name directive

CONF_DIR="/etc/nginx/sites-enabled"

echo "Updating Nginx configuration in $CONF_DIR..."

for file in $CONF_DIR/*; do
    if [ -f "$file" ]; then
        echo "Processing $file..."
        # Check if already exists
        if grep -q "client_max_body_size" "$file"; then
            echo "Updating existing client_max_body_size..."
            sed -i 's/client_max_body_size .*/client_max_body_size 0;/g' "$file"
        else
            echo "Adding client_max_body_size 0..."
            # Insert after 'server {' if possible, or after 'server_name ...;'
            # Let's try inserting after 'server_name' which is safer for Certbot configs
            sed -i '/server_name/a \    client_max_body_size 0;' "$file"
        fi
    fi
done

echo "Reloading Nginx..."
systemctl reload nginx
echo "Done."
