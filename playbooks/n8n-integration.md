# n8n AI Agent Integration Playbook

This playbook guides you through integrating osc-mcp with n8n AI Agent workflows for automated OBS package management.

## Overview

n8n is a workflow automation platform. By connecting osc-mcp as an MCP tool provider, you can create AI-powered workflows that automatically upgrade packages, fix build errors, and manage OBS projects.

The key workflow for package upgrades follows this 6-step sequence:
1. Branch the package from source project
2. Get current source files (_service, spec, etc.)
3. Update the _service file with new version/revision
4. **Delete old source archives** (using `osc rm`)
5. **Run OBS services** (`osc service runall`) to fetch new tarballs
6. Commit changes (auto-handles `osc add` for new files)

**Critical:** Steps 4 and 5 must be in this exact order - delete BEFORE running services.

## Prerequisites

- osc-mcp server running (see [setup-osc-mcp.md](setup-osc-mcp.md))
- n8n instance (self-hosted or cloud)
- OpenAI API key (for GPT-4o or similar)
- Required OBS service packages installed on osc-mcp host:
  ```bash
  zypper install obs-service-tar_scm obs-service-download_files \
                 obs-service-recompress obs-service-set_version
  ```

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Webhook        │────>│  Package Config │────>│  Build Prompt   │
│  Trigger        │     │  (Set Node)     │     │  (Code Node)    │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        v
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Output Result  │<────│   AI Agent      │<────│ OpenAI Model    │
│  (Set Node)     │     │  (LangChain)    │     │ (GPT-4o)        │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                               │
                               v
                        ┌─────────────────┐
                        │  osc-mcp Tools  │
                        │  (MCP Client)   │
                        └─────────────────┘
                               │
                               v
                        ┌─────────────────┐
                        │  osc-mcp Server │
                        │  (Port 8666)    │
                        └─────────────────┘
```

## Step 1: Network Configuration

### Local Development

If n8n and osc-mcp run on the same machine:

```
MCP Endpoint: http://localhost:8666/mcp
```

### Kubernetes Deployment

If n8n runs in Kubernetes and osc-mcp runs on the host:

1. Create a Service and Endpoints pointing to the host:

```yaml
# osc-mcp-service.yaml
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
      - ip: <HOST_IP>  # Replace with your host IP
    ports:
      - port: 8666
```

2. Apply the configuration:

```bash
kubectl apply -f osc-mcp-service.yaml
```

3. Use the internal service URL:

```
MCP Endpoint: http://osc-mcp.n8n.svc.cluster.local:8666/mcp
```

## Step 2: Configure n8n Credentials

### OpenAI Credentials

1. Go to n8n Settings > Credentials
2. Add new "OpenAI API" credential
3. Enter your API key
4. Test the connection

## Step 3: Create the Workflow

### Node 1: Webhook Trigger

- **Type:** Webhook
- **Path:** `/webhook/obs-upgrade`
- **Method:** POST
- **Response Mode:** When Last Node Finishes

### Node 2: Package Config (Set Node)

Extract parameters with defaults:

```javascript
{
  "packageName": "={{ $json.body?.packageName || 'buildkit' }}",
  "sourceProject": "={{ $json.body?.sourceProject || 'openSUSE:Factory' }}",
  "targetVersion": "={{ $json.body?.targetVersion || '0.27.0' }}"
}
```

### Node 3: Build Prompt (Code Node)

**Important:** The AI Agent node does not evaluate n8n expressions in the prompt text field. Use a Code node to construct the prompt with actual values:

```javascript
const pkg = $input.first().json.packageName;
const proj = $input.first().json.sourceProject;
const ver = $input.first().json.targetVersion;

return [{
  json: {
    packageName: pkg,
    sourceProject: proj,
    targetVersion: ver,
    prompt: `You are an OBS package maintainer assistant upgrading a package.

PACKAGE: ${pkg}
SOURCE_PROJECT: ${proj}
TARGET_VERSION: ${ver}

Execute these steps IN EXACT ORDER:

**Step 1: Branch the package**
Call branch_bundle with:
- project_name: "${proj}"
- bundle_name: "${pkg}"
Save the target_project and checkout_dir from the response.

**Step 2: Get source files**
Call list_source_files with:
- project_name: (target_project from step 1)
- package_name: "${pkg}"
- local: true
This returns the _service file and other source files.

**Step 3: Update _service file**
Call edit_file with:
- directory: (checkout_dir from step 1)
- filename: "_service"
- content: (complete _service file with revision updated to "v${ver}")
Update the <param name="revision"> value to the new version tag.

**Step 4: Delete old archives (CRITICAL - must be done BEFORE step 5)**
Call delete_files with:
- directory: (checkout_dir from step 1)
- patterns: ["*.tar.gz", "*.tar", "*.obscpio", "*.tar.xz", "*.tar.bz2"]
This runs 'osc rm' to mark old archives for removal.

**Step 5: Run services (AFTER step 4)**
Call run_services with:
- project_name: (target_project from step 1)
- bundle_name: "${pkg}"
Do NOT specify the services parameter - this runs ALL services to fetch the new tarball.

**Step 6: Commit changes**
Call commit with:
- directory: (checkout_dir from step 1)
- message: "Update ${pkg} to version ${ver}"
The commit automatically handles 'osc add' for new files.

After completing all steps, provide a summary with:
- Branch project name
- Link to OBS package page
- Commit revision number
`
  }
}];
```

### Node 4: OpenAI Chat Model

- **Model:** gpt-4o
- **Credentials:** Your OpenAI API credential

### Node 5: AI Agent (LangChain)

- **Type:** Tools Agent
- **Prompt:** `{{ $json.prompt }}`
- **System Message:** "You are an OBS package maintainer. Use the exact values given in the prompt for tool parameters."
- **Max Iterations:** 20

### Node 6: MCP Client Tool

- **Type:** MCP Client Tool (under Tools category)
- **SSE Endpoint:** `http://osc-mcp.n8n.svc.cluster.local:8666/mcp`
- **Transport:** httpStreamable
- **Authentication:** None
- **Timeout:** 600000 (10 minutes) - OBS services can take time for large repos

### Node 7: Output Result (Set Node)

```javascript
{
  "result": "={{ $json.output }}"
}
```

Connect the MCP Client Tool to the AI Agent as a tool.

## Step 4: Activate the Workflow

1. Save the workflow
2. Toggle the "Active" switch to enable
3. The webhook URL will be displayed

## Step 5: Test the Workflow

### Trigger a package upgrade:

```bash
curl -X POST "http://<n8n-host>:<port>/webhook/obs-upgrade" \
  -H "Content-Type: application/json" \
  -d '{
    "packageName": "buildkit",
    "sourceProject": "openSUSE:Factory",
    "targetVersion": "0.27.0"
  }'
```

### Expected response:

```json
{
  "result": "### Summary\n\n- **Branch Project:** `home:<username>:branches:openSUSE:Factory`\n- **OBS URL:** [link](https://build.opensuse.org/package/show/home:<username>:branches:openSUSE:Factory/buildkit)\n- **Revision:** 2\n\nThe package `buildkit` has been successfully updated to version 0.27.0."
}
```

## Timeout Settings

| Component | Timeout | Description |
|-----------|---------|-------------|
| Workflow Execution | 1800s (30 min) | Overall workflow timeout in n8n settings |
| MCP Client Tool | 600000ms (10 min) | Individual tool call timeout |
| AI Agent Max Iterations | 20 | Maximum tool call iterations |

To set workflow execution timeout: Workflow Settings > Execution Timeout > 1800

## Tool Call Sequence

The AI Agent executes tools in this **exact order**:

1. **branch_bundle** - Creates a branch and checks out the package locally
2. **list_source_files** - Retrieves _service, spec file, and other source contents
3. **edit_file** - Updates the revision/version in the _service file
4. **delete_files** - Removes old source archives using `osc rm` (BEFORE step 5!)
5. **run_services** - Runs `osc service runall` to fetch new tarballs (AFTER step 4!)
6. **commit** - Commits changes (auto-handles `osc add` for new files)

## Troubleshooting

### "Request timed out" in AI Agent

- Increase MCP Client Tool timeout to 600000ms (10 minutes)
- Set workflow execution timeout to 1800s (30 minutes) in Workflow Settings
- Check if `osc service runall` is taking too long (large repos with many files)

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
zypper install obs-service-tar_scm obs-service-download_files \
               obs-service-recompress obs-service-set_version
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

### AI returns template syntax literally

The AI Agent prompt field doesn't evaluate n8n expressions. Ensure you have the Build Prompt (Code) node constructing the prompt with actual values.

### "404 Not Found" errors from OBS

- Verify the package exists in the source project
- Check OBS credentials are valid
- Ensure the user has permission to branch

### SELinux blocking osc-mcp binary

If you see "Permission denied" when starting osc-mcp after deploying a new binary:
```bash
sudo restorecon -v /opt/osc-mcp/osc-mcp
```

## Tested Packages

| Package | Source Project | Original Version | Target Version | Result |
|---------|---------------|------------------|----------------|--------|
| buildkit | openSUSE:Factory | 0.26.3 | 0.27.0 | Default example |
| hello   | openSUSE:Factory | 2.12.2 | 2.13 | Success |
| bc      | openSUSE:Factory | 1.08.2 | 1.08.3 | Success |

## See Also

- [Main osc-mcp documentation](../README.md)
- [n8n AI Agent workflow details](../docs/n8n-ai-agent-workflow.md)
