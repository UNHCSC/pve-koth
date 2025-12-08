#!/bin/bash

# Install Nginx
apt-get install -y nginx

# Configure Nginx for the competition website
cat <<EOF > /etc/nginx/sites-available/competition_website
server {
    listen 80;
    server_name _;
    root /var/www/competition_website;
    index index.html index.htm;
    location / {
        try_files \$uri \$uri/ =404;
    }
}
EOF

ln -s /etc/nginx/sites-available/competition_website /etc/nginx/sites-enabled/
rm /etc/nginx/sites-enabled/default

# Create website directory and a sample index.html
mkdir -p /var/www/competition_website

# Use the env variable KOTH_PUBLIC_FOLDER to download the website files (Authorization cookie must be set to env KOTH_ACCESS_TOKEN)
curl -o /var/www/competition_website/index.html "$KOTH_PUBLIC_FOLDER/website.html" -b "Authorization=$KOTH_ACCESS_TOKEN"

if [ $? -ne 0 ]; then
    echo "Failed to download the competition website HTML file."
    exit 1
fi

# Set permissions
chown -R www-data:www-data /var/www/competition_website
chmod -R 755 /var/www/competition_website

# Restart Nginx to apply changes
systemctl restart nginx