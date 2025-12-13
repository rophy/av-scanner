#!/bin/bash
#
# vm-start.sh - Start an existing QEMU VM
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
STATE_FILE="$PROJECT_DIR/.vm-state"

if [ ! -f "$STATE_FILE" ]; then
    echo "No VM state found. Run vm-init.sh first."
    exit 1
fi

source "$STATE_FILE"

if [ "$HYPERVISOR" = "multipass" ]; then
    echo "Starting Multipass VM..."
    multipass start "$VM_NAME"
    echo "VM started. SSH: ssh ubuntu@$VM_IP"
    exit 0
fi

# QEMU VM
if [ -z "$QEMU_DIR" ]; then
    QEMU_DIR="$HOME/qemu-vms"
fi

if pgrep -f "qemu.*$VM_NAME" > /dev/null; then
    echo "VM is already running"
    exit 0
fi

if [ ! -f "$QEMU_DIR/$VM_NAME.qcow2" ]; then
    echo "VM disk not found. Run vm-init.sh first."
    exit 1
fi

echo "Starting QEMU VM..."

qemu-system-x86_64 \
    -name "$VM_NAME" \
    -machine accel=tcg \
    -cpu qemu64 \
    -m 4096 \
    -smp 2 \
    -drive file="$QEMU_DIR/$VM_NAME.qcow2",format=qcow2 \
    -netdev user,id=net0,hostfwd=tcp::${SSH_PORT}-:22,hostfwd=tcp::${API_PORT}-:3000 \
    -device virtio-net-pci,netdev=net0 \
    -nographic \
    -pidfile "$QEMU_DIR/$VM_NAME.pid" \
    > "$QEMU_DIR/$VM_NAME.log" 2>&1 &

echo $! > "$QEMU_DIR/$VM_NAME.pid"

echo "Waiting for SSH..."
for i in {1..60}; do
    if sshpass -p 'ubuntu' ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -p "$SSH_PORT" ubuntu@localhost "echo ok" &>/dev/null; then
        echo "VM ready. SSH: sshpass -p 'ubuntu' ssh -p $SSH_PORT ubuntu@localhost"
        exit 0
    fi
    echo -n "."
    sleep 5
done

echo "VM failed to start. Check: $QEMU_DIR/$VM_NAME.log"
exit 1
