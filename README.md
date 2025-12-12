# AV Scanner - Antivirus Abstraction Layer

A unified antivirus scanning service that supports multiple AV engines (ClamAV and Trend Micro DS Agent) running on a **dedicated VM** with a consistent API.

## Architecture

Both ClamAV and Trend Micro DS Agent run on a **dedicated Ubuntu VM**. Both engines share the **same interface**:

| Component | ClamAV | Trend Micro DS Agent |
|-----------|--------|----------------------|
| **RTS Log File** | `/var/log/clamav/clamonacc.log` | `/var/log/ds_agent/ds_agent.log` |
| **Scan Binary** | `/usr/bin/clamdscan` | `/opt/ds_agent/dsa_scan` |

```
┌─────────────────────────────────────────────────────────────┐
│                   SCANNING VM (Ubuntu)                      │
│                                                             │
│  ┌─────────────────┐         ┌─────────────────────────┐   │
│  │    ClamAV       │         │  Trend Micro DS Agent   │   │
│  │                 │         │                         │   │
│  │  Log file:      │         │  Log file:              │   │
│  │  clamonacc.log  │         │  ds_agent.log           │   │
│  │                 │         │                         │   │
│  │  Binary:        │         │  Binary:                │   │
│  │  clamdscan      │         │  dsa_scan               │   │
│  └────────┬────────┘         └────────────┬────────────┘   │
│           │                               │                 │
│           └───────────┬───────────────────┘                 │
│                       │                                     │
│  ┌────────────────────▼────────────────────────────────┐   │
│  │              AV Scanner Service                      │   │
│  │                                                      │   │
│  │  - RTS log file (monitors for scan results)          │   │
│  │  - Scan binary (executes for manual scans)           │   │
│  │  - Shared scan directory (/tmp/av-scanner)           │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Features

- **Unified interface**: Both engines use the same pattern (log file + binary)
- **Isolated VM**: AV engines run in a dedicated Ubuntu VM
- **RTS-first architecture**: Monitors RTS log for real-time scan results
- **Manual scan support**: Executes scan binary for on-demand scanning
- **Ephemeral file handling**: Files are deleted immediately after scanning

## Quick Start with Multipass

### 1. Install Multipass

```bash
# Ubuntu/Debian
sudo snap install multipass

# macOS
brew install multipass

# Windows
# Download from https://multipass.run/download/windows
```

### 2. Create the Scanning VM

```bash
# Create Ubuntu VM with 2 CPUs, 2GB RAM, 10GB disk
multipass launch --name av-scanner --cpus 2 --memory 2G --disk 10G 24.04

# Verify VM is running
multipass list
```

### 3. Install ClamAV in the VM

```bash
# Enter the VM
multipass shell av-scanner

# Inside VM: Install ClamAV
sudo apt-get update
sudo apt-get install -y clamav clamav-daemon

# Wait for freshclam to download virus definitions (may take a minute)
sudo systemctl stop clamav-freshclam
sudo freshclam
sudo systemctl start clamav-freshclam

# Start ClamAV daemon
sudo systemctl enable clamav-daemon
sudo systemctl start clamav-daemon

# Create scan directory
sudo mkdir -p /tmp/av-scanner
sudo chmod 777 /tmp/av-scanner

# Exit VM
exit
```

### 3a. (Optional) Enable On-Access Scanning (RTS)

On-access scanning uses `clamonacc` which is included with `clamav-daemon`.
It requires kernel fanotify support and additional configuration.

```bash
# Enter the VM
multipass shell av-scanner

# Configure clamd for on-access scanning
sudo tee -a /etc/clamav/clamd.conf << 'EOF'

# On-Access Scanning Configuration
OnAccessIncludePath /tmp/av-scanner
OnAccessExcludeUname clamav
OnAccessPrevention yes
OnAccessDisableDDD yes
EOF

# Restart clamd to apply changes
sudo systemctl restart clamav-daemon

# Create systemd service for clamonacc
sudo tee /etc/systemd/system/clamav-clamonacc.service << 'EOF'
[Unit]
Description=ClamAV On-Access Scanner
Requires=clamav-daemon.service
After=clamav-daemon.service

[Service]
Type=simple
User=root
ExecStart=/usr/sbin/clamonacc --foreground --log=/var/log/clamav/clamonacc.log --move=/tmp/quarantine
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Create quarantine and log directories
sudo mkdir -p /tmp/quarantine
sudo touch /var/log/clamav/clamonacc.log
sudo chown clamav:clamav /var/log/clamav/clamonacc.log

# Enable and start clamonacc
sudo systemctl daemon-reload
sudo systemctl enable clamav-clamonacc
sudo systemctl start clamav-clamonacc

# Verify it's running
sudo systemctl status clamav-clamonacc

# Exit VM
exit
```

See [ClamAV On-Access Scanning Documentation](https://docs.clamav.net/manual/OnAccess.html) for more details.

### 4. Install Node.js and Deploy Scanner Service

```bash
# Enter the VM
multipass shell av-scanner

# Install Node.js 20
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs

# Clone/copy your project to the VM
# Option A: Clone from git
git clone <your-repo> /home/ubuntu/av-scanner

# Option B: Transfer files from host
exit
multipass transfer -r ./av-scanner av-scanner:/home/ubuntu/
multipass shell av-scanner

# Build and start the service
cd /home/ubuntu/av-scanner
npm install
npm run build
npm start
```

### 5. Access the Scanner API

```bash
# Get VM IP address
multipass info av-scanner | grep IPv4

# Test the API (from host)
curl http://<VM_IP>:3000/api/v1/health

# Scan a file
curl -X POST -F "file=@testfile.txt" http://<VM_IP>:3000/api/v1/scan
```

## Multipass VM Management

```bash
# List VMs
multipass list

# Start/Stop VM
multipass start av-scanner
multipass stop av-scanner

# Shell into VM
multipass shell av-scanner

# Transfer files to VM
multipass transfer ./file.txt av-scanner:/home/ubuntu/

# Delete VM
multipass delete av-scanner
multipass purge
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AV_ENGINE` | clamav | Active engine (clamav/trendmicro) |
| `UPLOAD_DIR` | /tmp/av-scanner | Shared scan directory |
| `CLAMAV_RTS_LOG_PATH` | /var/log/clamav/clamonacc.log | ClamAV RTS log file |
| `CLAMAV_SCAN_BINARY_PATH` | /usr/bin/clamdscan | ClamAV scan binary |
| `TM_RTS_LOG_PATH` | /var/log/ds_agent/ds_agent.log | DS Agent RTS log file |
| `TM_SCAN_BINARY_PATH` | /opt/ds_agent/dsa_scan | DS Agent scan binary |

## API

### POST /api/v1/scan
Upload and scan a file.

```bash
curl -X POST -F "file=@testfile.txt" http://<VM_IP>:3000/api/v1/scan
```

**Response:**
```json
{
  "fileId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "clean",
  "engine": "clamav",
  "signature": null,
  "duration": 150
}
```

### GET /api/v1/health
Health check for all engines.

### GET /api/v1/engines
List available engines.

## Testing with EICAR

The EICAR test file must contain the exact 68-byte signature with no extra content.

```bash
# Download official EICAR test file
curl -s https://secure.eicar.org/eicar.com -o /tmp/eicar.com

# Test manual scan (inside VM)
clamdscan /tmp/eicar.com
# Expected: Win.Test.EICAR_HDB-1 FOUND

# Test on-access scanning (if enabled)
cp /tmp/eicar.com /tmp/av-scanner/
cat /tmp/av-scanner/eicar.com
# Expected: Operation not permitted (file blocked by clamonacc)

# Test via API
curl -X POST -F "file=@/tmp/eicar.com" http://<VM_IP>:3000/api/v1/scan
# Expected response: status = "infected", signature = "Win.Test.EICAR_HDB-1"
```

## License

MIT
