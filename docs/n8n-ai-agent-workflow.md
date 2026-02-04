# OBS Package Upgrade - n8n AI Agent Workflow

This document describes the n8n AI Agent workflow that automates OBS (Open Build Service) package version upgrades using the osc-mcp MCP server.

## Overview

The workflow uses an AI Agent (GPT-4o) connected to osc-mcp tools to:
1. Branch a package from a source project
2. Update the `_service` file with the new version/revision
3. Delete old source archives
4. Run OBS services to fetch new source tarballs
5. Commit the changes to the branched package

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Manual/Webhook │────▶│  Package Config │────▶│  Build Prompt   │
│  Trigger        │     │  (Set Node)     │     │  (Code Node)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Output Result  │◀────│   AI Agent      │◀────│ OpenAI Model    │
│  (Set Node)     │     │  (LangChain)    │     │ (GPT-4o)        │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌─────────────────┐
                        │  osc-mcp Tools  │
                        │  (MCP Client)   │
                        └─────────────────┘
                               │
                               ▼
                        ┌─────────────────┐
                        │  osc-mcp Server │
                        │  (Port 8666)    │
                        └─────────────────┘
```

## Components

### 1. Trigger
- **Manual Trigger** for testing, or
- **Webhook Trigger** for API access (`/webhook/obs-upgrade`)

### 2. Package Config (Set Node)
Configure the package to upgrade:
- `packageName` - The package to upgrade (e.g., "skopeo")
- `sourceProject` - Source project (e.g., "Virtualization:containers")
- `targetVersion` - Target version (e.g., "1.21.0")

### 3. Build Prompt (Code Node)
Constructs the AI prompt with actual values. This is necessary because the AI Agent node doesn't evaluate n8n expressions in the prompt text field.

### 4. AI Agent (LangChain)
Uses GPT-4o to orchestrate the upgrade process by calling osc-mcp tools in sequence.
- **Max Iterations:** 20
- **System Message:** "You are an OBS package maintainer. Use the exact values given in the prompt for tool parameters."

### 5. osc-mcp Tools (MCP Client)
Connects to the osc-mcp server and exposes these tools to the AI:
- `branch_bundle` - Branch a package and checkout locally
- `list_source_files` - Get source file contents (_service, spec, etc.)
- `edit_file` - Update _service or spec files
- `delete_files` - Remove old source archives (with `osc rm`)
- `run_services` - Run OBS services (`osc service runall`)
- `commit` - Commit changes (auto-handles `osc add` for new files)

## Tool Call Sequence (6 Steps)

The AI Agent executes tools in this **exact order**:

### Step 1: Branch the Package
```
branch_bundle:
  project_name: "Virtualization:containers"
  bundle_name: "skopeo"
```
Output: `target_project`, `checkout_dir`

### Step 2: Get Source Files
```
list_source_files:
  project_name: (target_project from step 1)
  package_name: "skopeo"
  local: true
```
Output: Contents of `_service`, spec file, and other source files

### Step 3: Update _service File
Update the revision/version parameter in the `_service` file:
```
edit_file:
  directory: (checkout_dir from step 1)
  filename: "_service"
  content: (complete _service file with updated revision)
```

### Step 4: Delete Old Archives (CRITICAL - Before Step 5)
**This step MUST be done BEFORE running services!**
```
delete_files:
  directory: (checkout_dir from step 1)
  patterns: ["*.tar.gz", "*.tar", "*.obscpio", "*.tar.xz", "*.tar.bz2"]
```
This runs `osc rm` on matching files to properly mark them for removal.

### Step 5: Run Services (After Step 4)
```
run_services:
  project_name: (target_project from step 1)
  bundle_name: "skopeo"
```
**Do NOT specify the `services` parameter** - this runs ALL services defined in the `_service` file.

### Step 6: Commit the Changes
```
commit:
  directory: (checkout_dir from step 1)
  message: "Update skopeo to version 1.21.0"
```
The commit automatically handles:
- Adding new files (the new tarball)
- Removing deleted files (the old tarball)

## Configuration

### Timeout Settings

| Component | Timeout | Description |
|-----------|---------|-------------|
| Workflow Execution | 1800s (30 min) | Overall workflow timeout |
| MCP Client Tool | 600000ms (10 min) | Individual tool call timeout |
| AI Agent Max Iterations | 20 | Maximum tool call iterations |

### osc-mcp Server Requirements

The osc-mcp server host must have these packages installed:
```bash
# For tar_scm service (git-based packages)
zypper install obs-service-tar_scm

# For download_files service (URL-based packages)
zypper install obs-service-download_files

# For recompress service
zypper install obs-service-recompress

# For set_version service
zypper install obs-service-set_version
```

### osc-mcp Server Configuration

The osc-mcp server must be configured with valid OBS credentials in `~/.config/osc/oscrc`:
```ini
[general]
apiurl=https://api.opensuse.org

[https://api.opensuse.org]
user=<username>
pass=<password>
credentials_mgr_class=osc.credentials.PlaintextConfigFileCredentialsManager
```

### Kubernetes Network Configuration

If n8n runs in Kubernetes and osc-mcp runs on the host:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: osc-mcp
  namespace: n8n
spec:
  ports:
  - port: 8666
    targetPort: 8666
---
apiVersion: v1
kind: Endpoints
metadata:
  name: osc-mcp
  namespace: n8n
subsets:
  - addresses:
      - ip: <osc-mcp-host-ip>
    ports:
      - port: 8666
```

MCP Client Tool endpoint: `http://osc-mcp.n8n.svc.cluster.local:8666/mcp`

## Workflow ID

- **n8n Workflow ID:** `wvK2eTmn2swcqUBJ`
- **Name:** "OBS Package Upgrade - AI Agent with osc-mcp"

## Example _service File

Packages using `tar_scm` service (git-based):
```xml
<services>
  <service name="tar_scm" mode="manual">
    <param name="url">https://github.com/containers/skopeo</param>
    <param name="scm">git</param>
    <param name="filename">skopeo</param>
    <param name="versionformat">@PARENT_TAG@</param>
    <param name="versionrewrite-pattern">v(.*)</param>
    <param name="revision">v1.21.0</param>  <!-- Update this -->
    <param name="changesgenerate">enable</param>
  </service>
  <service name="recompress" mode="manual">
    <param name="file">*.tar</param>
    <param name="compression">xz</param>
  </service>
  <service name="set_version" mode="manual">
    <param name="basename">skopeo</param>
  </service>
</services>
```

## Tested Packages

| Package | Source Project | From Version | To Version | Result |
|---------|---------------|--------------|------------|--------|
| skopeo | Virtualization:containers | 1.20.0 | 1.21.0 | In progress |
| conmon | devel:microos | 2.1.x | 2.2.0 | Tested |

## Troubleshooting

### "Request timed out" in AI Agent

- Increase MCP Client Tool timeout to 600000ms (10 minutes)
- Set workflow execution timeout to 1800s (30 minutes)
- Check if `osc service runall` is taking too long (large repos)

### "Could not connect to your MCP server"

- Verify osc-mcp is running: `systemctl status osc-mcp`
- Check Kubernetes Service/Endpoint configuration
- Test connectivity from n8n pod:
  ```bash
  kubectl exec -it <n8n-pod> -n n8n -- curl http://osc-mcp.n8n.svc.cluster.local:8666/mcp
  ```

### Services fail with "obs-service-xxx not found"

Install the required OBS service packages on the osc-mcp server host:
```bash
zypper install obs-service-tar_scm obs-service-download_files obs-service-recompress obs-service-set_version
```

### Old tarball not removed / New tarball not added

Ensure the workflow executes steps in the correct order:
1. `delete_files` MUST run BEFORE `run_services`
2. The `delete_files` tool uses `osc rm` to properly mark files for removal
3. The `commit` tool automatically runs `osc add` for new files

### AI executes steps in wrong order

The prompt must clearly emphasize step order with phrases like:
- "Execute these steps IN EXACT ORDER"
- "CRITICAL - must be done BEFORE step X"
- "This step MUST be done BEFORE running services"

### "404 Not Found" errors from OBS

- Verify package exists in source project
- Check OBS credentials are valid
- Ensure the user has permission to branch

## See Also

- [n8n Integration Playbook](../playbooks/n8n-integration.md)
- [osc-mcp README](../README.md)
