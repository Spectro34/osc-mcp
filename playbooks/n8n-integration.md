# n8n AI Agent Integration Playbook

This playbook guides you through integrating osc-mcp with n8n AI Agent workflows for automated OBS package management.

## Overview

n8n is a workflow automation platform. By connecting osc-mcp as an MCP tool provider, you can create AI-powered workflows that automatically upgrade packages, fix build errors, and manage OBS projects.

## Prerequisites

- osc-mcp server running (see [setup-osc-mcp.md](setup-osc-mcp.md))
- n8n instance (self-hosted or cloud)
- OpenAI API key (for GPT-4o or similar)

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

Your task is to upgrade the package to the target version by following these steps:

1. **Branch the package** using branch_bundle:
   - project_name: "${proj}"
   - bundle_name: "${pkg}"

2. **Get the spec file content** using list_source_files:
   - project_name: (use the target_project from step 1)
   - package_name: "${pkg}"
   - local: true

3. **Update the version** using edit_file:
   - directory: (use checkout_dir from step 1)
   - filename: "${pkg}.spec"
   - content: (modified spec file with Version: ${ver})

4. **Commit the changes** using commit:
   - directory: (same as step 3)
   - message: "Update to version ${ver}"

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
- **System Message:** "You are an OBS package maintainer assistant. Execute the steps exactly as described."

### Node 6: MCP Client Tool

- **Type:** MCP Client Tool (under Tools category)
- **SSE Endpoint:** `http://osc-mcp.n8n.svc.cluster.local:8666/mcp`
- **Transport:** httpStreamable
- **Authentication:** None
- **Timeout:** 180000 (3 minutes)

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

## Tool Call Sequence

The AI Agent executes tools in this order:

1. **branch_bundle** - Creates a branch and checks out the package
2. **list_source_files** - Retrieves the current spec file content
3. **edit_file** - Updates the Version field in the spec
4. **commit** - Commits the changes to OBS

## Troubleshooting

### "Could not connect to your MCP server"

- Verify osc-mcp is running: `systemctl status osc-mcp`
- Check Kubernetes Service/Endpoint configuration
- Test connectivity from n8n pod:
  ```bash
  kubectl exec -it <n8n-pod> -n n8n -- curl http://osc-mcp.n8n.svc.cluster.local:8666/mcp
  ```

### AI returns template syntax literally

The AI Agent prompt field doesn't evaluate n8n expressions. Ensure you have the Build Prompt (Code) node constructing the prompt with actual values.

### "404 Not Found" errors from OBS

- Verify the package exists in the source project
- Check OBS credentials are valid
- Ensure the user has permission to branch

### Timeout errors

Increase the MCP Client Tool timeout. OBS operations can take time, especially for large packages.

## Tested Packages

| Package | Source Project | Original Version | Target Version | Result |
|---------|---------------|------------------|----------------|--------|
| buildkit | openSUSE:Factory | 0.26.3 | 0.27.0 | Default example |
| hello   | openSUSE:Factory | 2.12.2 | 2.13 | Success |
| bc      | openSUSE:Factory | 1.08.2 | 1.08.3 | Success |

## See Also

- [Main osc-mcp documentation](../README.md)
- [n8n AI Agent workflow details](../docs/n8n-ai-agent-workflow.md)
