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
│  │         AV Scanner Service (Podman + Quadlet)        │   │
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
- **Systemd integration**: Container managed via Podman Quadlet

## Quick Start

### 1. Install Prerequisites

```bash
# Install Multipass (Ubuntu/Debian)
sudo snap install multipass

# Install Ansible (in a virtualenv named 'venv' - required by Makefile)
python3 -m venv venv
source venv/bin/activate
pip install ansible
```

> **Note:** The `venv` directory must exist with Ansible installed. `make deploy` automatically activates it.

### 2. Create the Scanning VM

```bash
# Create Ubuntu VM with 2 CPUs, 2GB RAM, 10GB disk
multipass launch --name av-scanner --cpus 2 --memory 2G --disk 10G 24.04

# Verify VM is running
multipass list
```

### 3. Build and Deploy

```bash
# Get VM IP address
export AV_SCANNER_IP=$(multipass info av-scanner | grep IPv4 | awk '{print $2}')

# Copy your SSH key to the VM
multipass exec av-scanner -- bash -c "mkdir -p ~/.ssh && chmod 700 ~/.ssh"
cat ~/.ssh/id_rsa.pub | multipass exec av-scanner -- bash -c "cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"

# Build the Docker image locally and deploy to VM
make deploy
```

The `make deploy` command will:
1. Build the Docker image locally
2. Save it to a tarball
3. Transfer the image to the VM via Ansible
4. Load the image into Podman on the VM
5. Create a Quadlet systemd service
6. Start and enable the service
7. Wait for health check to pass

### 4. Access the Scanner API

```bash
# Get VM IP address
multipass info av-scanner | grep IPv4

# Test the API (from host)
curl http://<VM_IP>:3000/api/v1/health

# Scan a file
curl -X POST -F "file=@testfile.txt" http://<VM_IP>:3000/api/v1/scan
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the Docker image locally |
| `make deploy` | Build, save, and deploy image to VM |
| `make clean` | Remove local image and tarball |

## Service Management

The scanner runs as a systemd service via Podman Quadlet:

```bash
# SSH into VM
multipass shell av-scanner

# Check service status
sudo systemctl status av-scanner

# View logs
sudo journalctl -u av-scanner -f

# Restart service
sudo systemctl restart av-scanner

# Stop service
sudo systemctl stop av-scanner
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

To change configuration, edit the Quadlet file on the VM:

```bash
sudo vi /etc/containers/systemd/av-scanner.container
sudo systemctl daemon-reload
sudo systemctl restart av-scanner
```

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
multipass exec av-scanner -- clamdscan /tmp/eicar.com
# Expected: Win.Test.EICAR_HDB-1 FOUND

# Test via API
curl -X POST -F "file=@/tmp/eicar.com" http://<VM_IP>:3000/api/v1/scan
# Expected response: status = "infected", signature = "Win.Test.EICAR_HDB-1"
```

## License

MIT
