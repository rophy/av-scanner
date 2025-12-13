## Git Commit Policy (MANDATORY)

**Commit message format:**
```
<type>: <short description>

[optional body explaining why/what changed]
```

**RULES:**
- NO "Generated with Claude Code" footer
- NO "Co-Authored-By: Claude" line
- NO mention of "Claude" or "Happy" anywhere
- Keep messages short (1-5 lines preferred)
- Types: feat, fix, refactor, chore, docs, build, test

## VM Management

**Before creating a VM:**
1. Check KVM support: `ls /dev/kvm` (exists = KVM available)
2. Check Multipass: `multipass list` (shows existing VMs)
3. Check QEMU VMs: `cat .vm-state` or `ls ~/qemu-vms/`
4. Stop existing VM first: `make vm-stop` and `rm .vm-state`

**Interactive prompts:** `make vm-init` and `./scripts/vm-init.sh` have interactive prompts. When running non-interactively, pipe `echo "y"` or use:
- `HYPERVISOR=multipass make vm-init` - skip hypervisor prompt
- `HYPERVISOR=qemu make vm-init` - skip hypervisor prompt

**Performance:**
- Multipass (KVM): ~1-2 min VM startup, recommended for development
- QEMU (TCG): ~5-10 min VM startup, software emulation fallback when KVM unavailable

**Port conflicts:** Default API_PORT=3000. If occupied, use `API_PORT=3001 make vm-init`

## Container Runtime

This project uses **podman** (not docker) for:
- Building images: `podman build`
- Pushing to registry: `podman push --tls-verify=false`
- Dockerfile must use fully qualified image names (e.g., `docker.io/library/golang:1.23-alpine`)

## EICAR Test String

**NEVER** store the EICAR test string directly in source files - it will trigger local AV scanners (including TrendMicro which detects base64-encoded EICAR).

Store with one character replaced, fix at runtime:
```go
// 'O' at position 2 replaced with 'x'
broken := "X5x!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"
eicar := strings.Replace(broken, "x", "O", 1)
```

```bash
echo 'X5x!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*' | sed 's/x/O/'
```
