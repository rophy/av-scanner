#!/bin/bash
#
# vm-init.sh - Initialize AV Scanner VM with appropriate hypervisor
#
# Detects available hypervisor (Multipass with KVM or QEMU TCG) and creates
# an Ubuntu VM with SSH access.
#
# Usage:
#   ./vm-init.sh                    # Auto-detect or prompt for hypervisor
#   ./vm-init.sh --hypervisor qemu  # Force QEMU (even if Multipass available)
#   ./vm-init.sh --hypervisor multipass
#

set -e

# Parse command line arguments
FORCE_HYPERVISOR=""
while [[ $# -gt 0 ]]; do
    case $1 in
        --hypervisor|-h)
            FORCE_HYPERVISOR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [--hypervisor multipass|qemu]"
            exit 1
            ;;
    esac
done

# Configuration
VM_NAME="${VM_NAME:-av-scanner}"
VM_MEMORY="${VM_MEMORY:-4096}"  # 4GB
VM_CPUS="${VM_CPUS:-2}"
VM_DISK="${VM_DISK:-10G}"
UBUNTU_VERSION="${UBUNTU_VERSION:-22.04}"
SSH_PORT="${SSH_PORT:-2222}"
API_PORT="${API_PORT:-3000}"
REGISTRY_PORT="${REGISTRY_PORT:-5000}"

# Directory setup
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
QEMU_DIR="${QEMU_DIR:-$HOME/qemu-vms}"

# SSH key for QEMU VMs (project-local key)
SSH_KEY_DIR="$PROJECT_DIR/.ssh"
SSH_KEY="$SSH_KEY_DIR/id_ed25519"

# State file to track hypervisor type
STATE_FILE="$PROJECT_DIR/.vm-state"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }

# Detect if KVM is available
detect_kvm() {
    [ -e /dev/kvm ] && [ -r /dev/kvm ] && [ -w /dev/kvm ] && return 0
    command -v kvm-ok &>/dev/null && kvm-ok &>/dev/null && return 0
    grep -qE '(vmx|svm)' /proc/cpuinfo 2>/dev/null && return 0
    return 1
}

# Detect available hypervisor
detect_hypervisor() {
    if detect_kvm && command -v multipass &>/dev/null; then
        echo "multipass"
    elif command -v qemu-system-x86_64 &>/dev/null; then
        echo "qemu-tcg"
    else
        echo "none"
    fi
}

# Install QEMU prerequisites
install_qemu_prerequisites() {
    log_info "Installing QEMU prerequisites..."
    sudo apt-get update
    sudo apt-get install -y qemu-system-x86 qemu-utils cloud-image-utils
    log_success "QEMU prerequisites installed"
}

# Generate SSH key if it doesn't exist
ensure_ssh_key() {
    if [ ! -f "$SSH_KEY" ]; then
        log_info "Generating SSH key for QEMU VMs..."
        mkdir -p "$SSH_KEY_DIR"
        ssh-keygen -t ed25519 -f "$SSH_KEY" -N "" -C "av-scanner-vm" >/dev/null
        log_success "SSH key generated: $SSH_KEY"
    fi
}

# Try SSH connection using project SSH key
try_ssh() {
    local port=$1
    local host=${2:-localhost}
    local user=${3:-ubuntu}
    local cmd=${4:-echo ok}

    ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
        -o ConnectTimeout=5 -o BatchMode=yes \
        -i "$SSH_KEY" \
        -p "$port" "$user@$host" "$cmd" &>/dev/null
}

# Create VM using Multipass
create_multipass_vm() {
    log_info "Creating VM with Multipass..."

    if multipass info "$VM_NAME" &>/dev/null; then
        log_warn "VM '$VM_NAME' already exists"
        read -p "Delete and recreate? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            multipass delete "$VM_NAME" --purge
        else
            log_info "Using existing VM"
            local vm_ip=$(multipass info "$VM_NAME" --format csv | tail -1 | cut -d, -f3)
            save_state "multipass" "$vm_ip" "22" ""
            return 0
        fi
    fi

    multipass launch \
        --name "$VM_NAME" \
        --cpus "$VM_CPUS" \
        --memory "${VM_MEMORY}M" \
        --disk "$VM_DISK" \
        "$UBUNTU_VERSION"

    local vm_ip=$(multipass info "$VM_NAME" --format csv | tail -1 | cut -d, -f3)

    # Set up SSH key
    log_info "Setting up SSH access..."
    if [ -f "$HOME/.ssh/id_rsa.pub" ]; then
        multipass exec "$VM_NAME" -- bash -c "mkdir -p ~/.ssh && chmod 700 ~/.ssh"
        cat "$HOME/.ssh/id_rsa.pub" | multipass exec "$VM_NAME" -- bash -c "cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
        log_success "SSH key configured"
    else
        log_warn "No SSH key found at ~/.ssh/id_rsa.pub"
    fi

    save_state "multipass" "$vm_ip" "22" ""
    log_success "Multipass VM created at $vm_ip"
}

# Create VM using QEMU TCG
create_qemu_vm() {
    log_info "Creating VM with QEMU (TCG mode)..."
    log_warn "TCG mode is slower than KVM but works without hardware virtualization"

    mkdir -p "$QEMU_DIR"

    if [ -f "$QEMU_DIR/$VM_NAME.qcow2" ]; then
        log_warn "VM disk already exists"
        read -p "Delete and recreate? [y/N] " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            pkill -f "qemu.*$VM_NAME" 2>/dev/null || true
            rm -f "$QEMU_DIR/$VM_NAME.qcow2" "$QEMU_DIR/$VM_NAME-seed.img"
        else
            log_info "Using existing VM disk"
        fi
    fi

    # Download Ubuntu cloud image if needed
    local base_image="$QEMU_DIR/ubuntu-${UBUNTU_VERSION}-base.img"
    if [ ! -f "$base_image" ]; then
        log_info "Downloading Ubuntu $UBUNTU_VERSION cloud image..."
        local image_url
        case $UBUNTU_VERSION in
            24.04) image_url="https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img" ;;
            22.04) image_url="https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img" ;;
            *)     image_url="https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img" ;;
        esac
        wget -q --show-progress -O "$base_image" "$image_url"
        log_success "Cloud image downloaded"
    fi

    # Create VM disk
    if [ ! -f "$QEMU_DIR/$VM_NAME.qcow2" ]; then
        qemu-img create -F qcow2 -b "$base_image" -f qcow2 "$QEMU_DIR/$VM_NAME.qcow2" "$VM_DISK"
    fi

    # Ensure SSH key exists
    ensure_ssh_key
    local ssh_pub_key=$(cat "$SSH_KEY.pub")

    # Create cloud-init config (SSH key auth only, no password)
    cat > "$QEMU_DIR/user-data" << EOF
#cloud-config
hostname: $VM_NAME
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: true
    ssh_authorized_keys:
      - $ssh_pub_key
ssh_pwauth: false
packages:
  - python3
  - python3-apt
EOF

    cat > "$QEMU_DIR/meta-data" << EOF
instance-id: ${VM_NAME}-001
local-hostname: $VM_NAME
EOF

    cloud-localds "$QEMU_DIR/$VM_NAME-seed.img" "$QEMU_DIR/user-data" "$QEMU_DIR/meta-data"

    # Start QEMU
    log_info "Starting QEMU VM (boot takes 3-5 minutes with TCG)..."

    qemu-system-x86_64 \
        -name "$VM_NAME" \
        -machine accel=tcg \
        -cpu qemu64 \
        -m "$VM_MEMORY" \
        -smp "$VM_CPUS" \
        -drive file="$QEMU_DIR/$VM_NAME.qcow2",format=qcow2 \
        -drive file="$QEMU_DIR/$VM_NAME-seed.img",format=raw \
        -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22,hostfwd=tcp::${API_PORT}-:3000,hostfwd=tcp::${REGISTRY_PORT}-:5000 \
        -device virtio-net-pci,netdev=net0 \
        -nographic \
        -pidfile "$QEMU_DIR/$VM_NAME.pid" \
        > "$QEMU_DIR/$VM_NAME.log" 2>&1 &

    echo $! > "$QEMU_DIR/$VM_NAME.pid"

    # Wait for SSH
    log_info "Waiting for VM to boot..."
    local attempt=0
    while [ $attempt -lt 60 ]; do
        if try_ssh "$SSH_PORT"; then
            break
        fi
        echo -n "."
        sleep 5
        attempt=$((attempt + 1))
    done
    echo

    if [ $attempt -eq 60 ]; then
        log_error "VM failed to boot. Check: $QEMU_DIR/$VM_NAME.log"
        return 1
    fi

    # Wait for cloud-init
    log_info "Waiting for cloud-init..."
    try_ssh "$SSH_PORT" "localhost" "ubuntu" "cloud-init status --wait" || true

    save_state "qemu-tcg" "localhost" "$SSH_PORT" "ubuntu"
    log_success "QEMU VM created"
}

# Save VM state
save_state() {
    local hypervisor=$1 vm_ip=$2 ssh_port=$3 ssh_pass=$4

    cat > "$STATE_FILE" << EOF
HYPERVISOR=$hypervisor
VM_NAME=$VM_NAME
VM_IP=$vm_ip
SSH_USER=ubuntu
SSH_PORT=$ssh_port
SSH_PASS=$ssh_pass
API_PORT=$API_PORT
REGISTRY_PORT=$REGISTRY_PORT
QEMU_DIR=$QEMU_DIR
VM_MEMORY=$VM_MEMORY
VM_CPUS=$VM_CPUS
EOF
}

# Print connection info
print_info() {
    source "$STATE_FILE"

    echo
    echo "============================================"
    echo "  VM Ready: $VM_NAME"
    echo "============================================"
    echo "Hypervisor: $HYPERVISOR"
    echo

    if [ "$HYPERVISOR" = "multipass" ]; then
        echo "SSH:  ssh ubuntu@$VM_IP"
        echo "API:  http://$VM_IP:3000"
        echo
        echo "Management:"
        echo "  multipass shell $VM_NAME"
        echo "  multipass stop $VM_NAME"
        echo "  multipass delete $VM_NAME --purge"
    else
        echo "SSH:  ssh -i $SSH_KEY -p $SSH_PORT ubuntu@localhost"
        echo "API:  http://localhost:$API_PORT"
        echo
        echo "Management:"
        echo "  make vm-stop   # stop VM"
        echo "  make vm-start  # start VM"
    fi

    echo
    echo "Next steps:"
    echo "  1. Install ClamAV:  make setup-vm"
    echo "  2. Deploy scanner:  make deploy"
    echo "============================================"
}

# Check what hypervisors are available
check_available_hypervisors() {
    local available=""
    if detect_kvm && command -v multipass &>/dev/null; then
        available="multipass"
    fi
    if command -v qemu-system-x86_64 &>/dev/null; then
        [ -n "$available" ] && available="$available qemu" || available="qemu"
    fi
    echo "$available"
}

# Main
main() {
    echo
    echo "=== AV Scanner VM Init ==="
    echo

    local available=$(check_available_hypervisors)
    local hypervisor=""

    # Handle forced hypervisor selection
    if [ -n "$FORCE_HYPERVISOR" ]; then
        case $FORCE_HYPERVISOR in
            multipass)
                if ! detect_kvm || ! command -v multipass &>/dev/null; then
                    log_error "Multipass not available (requires KVM and multipass installed)"
                    exit 1
                fi
                hypervisor="multipass"
                ;;
            qemu|qemu-tcg)
                if ! command -v qemu-system-x86_64 &>/dev/null; then
                    log_error "QEMU not installed. Install with: sudo apt install qemu-system-x86 qemu-utils cloud-image-utils"
                    exit 1
                fi
                hypervisor="qemu-tcg"
                ;;
            *)
                log_error "Unknown hypervisor: $FORCE_HYPERVISOR (use 'multipass' or 'qemu')"
                exit 1
                ;;
        esac
    elif [ -z "$available" ]; then
        log_error "No hypervisor found. Install one of:"
        echo "  - Multipass: sudo snap install multipass"
        echo "  - QEMU: sudo apt install qemu-system-x86 qemu-utils cloud-image-utils"
        exit 1
    elif [[ "$available" == "multipass qemu" ]]; then
        # Both available - let user choose
        echo "Available hypervisors:"
        echo "  1) Multipass (KVM) - faster, recommended"
        echo "  2) QEMU (TCG)      - slower, works without KVM"
        echo
        read -p "Select hypervisor [1/2]: " -n 1 -r
        echo
        case $REPLY in
            2) hypervisor="qemu-tcg" ;;
            *) hypervisor="multipass" ;;
        esac
    elif [[ "$available" == *"multipass"* ]]; then
        hypervisor="multipass"
    else
        hypervisor="qemu-tcg"
    fi

    case $hypervisor in
        multipass)  log_success "Using Multipass (KVM)" ;;
        qemu-tcg)   log_warn "Using QEMU TCG (software emulation - slower)" ;;
    esac

    echo
    echo "VM: $VM_NAME | Memory: ${VM_MEMORY}MB | CPUs: $VM_CPUS | Disk: $VM_DISK"
    echo

    read -p "Create VM? [Y/n] " -n 1 -r
    echo
    [[ $REPLY =~ ^[Nn]$ ]] && exit 0

    # Check/install prerequisites for QEMU
    if [ "$hypervisor" = "qemu-tcg" ]; then
        if ! command -v cloud-localds &>/dev/null; then
            read -p "Install QEMU prerequisites? [Y/n] " -n 1 -r
            echo
            [[ ! $REPLY =~ ^[Nn]$ ]] && install_qemu_prerequisites
        fi
    fi

    # Create VM
    case $hypervisor in
        multipass) create_multipass_vm ;;
        qemu-tcg)  create_qemu_vm ;;
    esac

    print_info
}

main "$@"
