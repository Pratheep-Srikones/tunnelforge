# TunnelForge Self-Hosting & Deployment Guide

This guide provides step-by-step instructions for deploying a production-ready TunnelForge server on a Linux VPS (Oracle Cloud Free Tier, DigitalOcean, Hetzner, AWS, etc.) with automatic wildcard SSL certificates, Nginx reverse proxying, systemd service management, and firewall rules.

---

## 📋 Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Step 1: Domain & DNS Setup (Cloudflare)](#step-1-domain--dns-setup-cloudflare)
3. [Step 2: Build & Install Server Binary](#step-2-build--install-server-binary)
4. [Step 3: Systemd Service Installation](#step-3-systemd-service-installation)
5. [Step 4: Wildcard SSL Certificate (Certbot)](#step-4-wildcard-ssl-certificate-certbot)
6. [Step 5: Nginx Reverse Proxy & SELinux Setup](#step-5-nginx-reverse-proxy--selinux-setup)
7. [Step 6: VPS & Cloud Firewall Rules](#step-6-vps--cloud-firewall-rules)
8. [Step 7: Verifying Your Deployment](#step-7-verifying-your-deployment)
9. [Troubleshooting Common Issues](#troubleshooting-common-issues)

---

## 1. Prerequisites

* A Linux VPS running **Ubuntu 22.04 LTS** or **Oracle Linux / RHEL 8+** (1 OCPU, 1GB+ RAM is plenty).
* A public static IP address assigned to your VPS (e.g., `141.X.X.X`).
* A domain name managed via Cloudflare or another DNS provider (e.g., `tunnels.yourdomain.com`).
* SSH access to your VPS with `sudo` privileges.

---

## Step 1: Domain & DNS Setup (Cloudflare)

To support dynamic subdomains (like `my-app.tunnels.yourdomain.com`), you need both a root record and a wildcard A-record pointing to your VPS IP.

In your Cloudflare DNS dashboard, add two **A Records**:

| Type | Name | IPv4 Address | Proxy Status |
| :--- | :--- | :--- | :--- |
| **A** | `tunnels` | `<YOUR_VPS_IP>` | ⚪ **DNS Only** (Grey Cloud) |
| **A** | `*.tunnels` | `<YOUR_VPS_IP>` | ⚪ **DNS Only** (Grey Cloud) |

> ⚠️ **IMPORTANT**: Ensure Proxy Status is set to **DNS Only** (Grey Cloud). If set to Proxied (Orange Cloud), Cloudflare will interfere with Let's Encrypt DNS challenges and raw WebSocket tunnel connections.

---

## Step 2: Build & Install Server Binary

### 1. Build Server Binary

On your local development machine:

```bash
# Build for Linux ARM64 (Oracle Cloud Ampere)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o tunnelforge-server ./server

# Or for Linux AMD64 (Intel/AMD VPS)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o tunnelforge-server ./server
```

### 2. Copy Binary to VPS

```bash
scp tunnelforge-server opc@<YOUR_VPS_IP>:/tmp/
```

### 3. Install on VPS

SSH into your VPS and install the binary:

```bash
# Move binary to system path
sudo mv /tmp/tunnelforge-server /usr/local/bin/
sudo chmod +x /usr/local/bin/tunnelforge-server

# Create dedicated system user
sudo useradd --system --no-create-home --shell /bin/false tunnelforge 2>/dev/null || true

# Create config directory
sudo mkdir -p /etc/tunnelforge
sudo chown -R tunnelforge:tunnelforge /etc/tunnelforge
```

---

## Step 3: Systemd Service Installation

Install the systemd unit file from [`deploy/server.service`](deploy/server.service) so TunnelForge runs automatically on boot and restarts if interrupted:

```bash
# 1. Copy service file
sudo cp deploy/server.service /etc/systemd/system/tunnelforge.service

# 2. Set custom enrollment key in service file (Recommended for production)
sudo systemctl edit tunnelforge --drop-in=env.conf
```

Add the following override content:

```ini
[Service]
Environment="FORGE_ENROLLMENT_KEY=your_secure_custom_key_123"
```

```bash
# 3. Reload systemd
sudo systemctl daemon-reload

# 4. Enable and start TunnelForge
sudo systemctl enable --now tunnelforge

# 5. Check service status
sudo systemctl status tunnelforge
```

View live logs at any time with:

```bash
journalctl -u tunnelforge -f
```

---

## Step 4: Wildcard SSL Certificate (Certbot)

Wildcard certificates (`*.tunnels.yourdomain.com`) require a DNS-01 challenge.

Run Certbot on your VPS:

```bash
# Install certbot
sudo apt update && sudo apt install -y certbot    # On Ubuntu/Debian
# OR
sudo dnf install -y certbot                      # On Oracle Linux/RHEL

# Request wildcard certificate
sudo certbot certonly --manual --preferred-challenges dns \
  -d "tunnels.yourdomain.com" \
  -d "*.tunnels.yourdomain.com"
```

1. Certbot will output a TXT record (e.g., `_acme-challenge.tunnels`).
2. Go to Cloudflare DNS $\rightarrow$ **Add Record** $\rightarrow$ Type: **TXT** $\rightarrow$ Name: `_acme-challenge.tunnels` $\rightarrow$ Content: the string provided by certbot.
3. Wait 10 seconds, then press Enter in Certbot.
4. Certificate files will be generated at:
   * `/etc/letsencrypt/live/tunnels.yourdomain.com/fullchain.pem`
   * `/etc/letsencrypt/live/tunnels.yourdomain.com/privkey.pem`

---

## Step 5: Nginx Reverse Proxy & SELinux Setup

### 1. Nginx Configuration

Copy [`deploy/nginx.conf`](deploy/nginx.conf) to your Nginx configuration directory and update the domain names:

#### On Ubuntu / Debian

```bash
sudo cp deploy/nginx.conf /etc/nginx/sites-available/tunnelforge
sudo ln -sf /etc/nginx/sites-available/tunnelforge /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
```

#### On Oracle Linux / RHEL / CentOS

On RHEL-based distributions, Nginx reads configuration files from `/etc/nginx/conf.d/*.conf`:

```bash
sudo cp deploy/nginx.conf /etc/nginx/conf.d/tunnelforge.conf
```

### 2. Configure Domain in Nginx Config

Edit `/etc/nginx/conf.d/tunnelforge.conf` (or `/etc/nginx/sites-available/tunnelforge`) and replace `tunnels.yourdomain.com` with your actual domain:

```nginx
server {
    listen 80;
    server_name tunnels.yourdomain.com *.tunnels.yourdomain.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tunnels.yourdomain.com *.tunnels.yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/tunnels.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/tunnels.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_http_version 1.1;

        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (for fallback tunnels at /tunnel)
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;

        proxy_buffering off;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

### 3. SELinux Permission (Oracle Linux / RHEL Only)

> ⚠️ **CRITICAL FOR RHEL / ORACLE LINUX**: SELinux forbids Nginx from opening network connections to local ports by default, causing `502 Bad Gateway`. Enable network connect permission:

```bash
sudo setsebool -P httpd_can_network_connect 1
```

### 4. Test and Start Nginx

```bash
sudo nginx -t
sudo systemctl restart nginx
```

---

## Step 6: VPS & Cloud Firewall Rules

You must allow incoming traffic on ports **`80`** (HTTP), **`443`** (HTTPS), and **`7000`** (Raw TLS).

### 1. OS-level Firewall Rules

#### On Ubuntu (iptables / ufw)

```bash
sudo iptables -I INPUT 6 -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT 6 -p tcp --dport 7000 -j ACCEPT
sudo netfilter-persistent save
```

#### On Oracle Linux / RHEL (`firewalld`)

```bash
sudo firewall-cmd --add-port=80/tcp --add-port=443/tcp --add-port=7000/tcp --permanent
sudo firewall-cmd --reload
```

### 2. Cloud Provider Ingress Rules (Oracle Cloud / AWS / GCP)

If using Oracle Cloud (OCI), AWS Security Groups, or GCP Firewall, add **Ingress Rules** in your cloud console:

* **Source CIDR**: `0.0.0.0/0`
* **IP Protocol**: `TCP`
* **Destination Ports**: `80, 443, 7000`

---

## Step 7: Verifying Your Deployment

### 1. Verify Ports on VPS

Run on your VPS:

```bash
sudo ss -tulpn | grep -E ':(80|443|7000|8000)'
```

You should see:

* `0.0.0.0:80` (Nginx)
* `0.0.0.0:443` (Nginx)
* `*:7000` (tunnelforge-server)
* `*:8000` (tunnelforge-server)

### 2. Register Agent from Your Laptop

```bash
forge register --server tunnels.yourdomain.com
```

Output:

```text
[Register] Success!
Token: ********
Agent ID: 0e73ebec-5108-48a6-a627-39e4ce3b21b9
Config saved.
```

### 3. Run Tunnels

```bash
forge up -c tunnels.yaml
```

Access your public tunnels at:
`https://web.tunnels.yourdomain.com`

---

## Troubleshooting Common Issues

### Issue: `502 Bad Gateway` from Nginx

* **Cause**: SELinux is blocking Nginx from forwarding traffic to `127.0.0.1:8000`.
* **Fix**: Run `sudo setsebool -P httpd_can_network_connect 1`.

### Issue: `connect: no route to host`

* **Cause**: VPS `iptables` or cloud security list is blocking port 80/443/7000.
* **Fix**: Run `sudo iptables -I INPUT 6 -p tcp --dport 443 -j ACCEPT` and add Ingress rules in Cloud Console.

### Issue: `connect: connection refused`

* **Cause**: Packet reached VPS, but no service is listening on that port.
* **Fix**: Ensure Nginx is running (`sudo systemctl status nginx`) and TunnelForge service is active (`sudo systemctl status tunnelforge`).
