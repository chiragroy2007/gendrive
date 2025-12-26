#!/bin/bash
# GenDrive SSL Certificate Installation Script
# Run this script AFTER configuring DNS for drive.chirag404.me

set -e

echo "=== GenDrive SSL Certificate Installation ==="
echo ""

# Check if DNS is configured
echo "Checking DNS configuration for drive.chirag404.me..."
if ! nslookup drive.chirag404.me > /dev/null 2>&1; then
    echo "❌ ERROR: DNS record for drive.chirag404.me not found!"
    echo ""
    echo "Please configure your DNS first:"
    echo "  1. Add an A record for drive.chirag404.me pointing to your server's IP"
    echo "  2. Wait for DNS propagation (usually 5-30 minutes)"
    echo "  3. Run this script again"
    exit 1
fi

echo "✅ DNS record found!"
echo ""

# Obtain SSL certificate
echo "Obtaining SSL certificate from Let's Encrypt..."
certbot --nginx -d drive.chirag404.me --non-interactive --agree-tos --email chiragroy2007@gmail.com

if [ $? -eq 0 ]; then
    echo ""
    echo "✅ SSL certificate installed successfully!"
    echo ""
    echo "Your GenDrive server is now accessible at:"
    echo "  https://drive.chirag404.me"
    echo ""
    echo "Certificate will auto-renew. Test renewal with:"
    echo "  certbot renew --dry-run"
else
    echo ""
    echo "❌ SSL certificate installation failed!"
    echo "Check logs at: /var/log/letsencrypt/letsencrypt.log"
    exit 1
fi
