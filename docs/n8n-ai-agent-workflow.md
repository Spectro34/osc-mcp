# OBS Package Upgrade - n8n AI Agent Workflow

This document describes the n8n AI Agent workflow that automates OBS (Open Build Service) package version upgrades using the osc-mcp MCP server.

## Overview

The workflow uses an AI Agent (GPT-4o) connected to osc-mcp tools to:
1. Branch a package from a source project
2. Update the spec file with a new version
3. Commit the changes to the branched package

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Webhook        │────▶│  Package Config │────▶│  Build Prompt   │
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

### 1. Webhook Trigger
- **Path:** `/webhook/obs-upgrade`
- **Method:** POST
- **Content-Type:** application/json

### 2. Package Config (Set Node)
Extracts parameters from the webhook request body with defaults:
- `packageName` - The package to upgrade (default: "hello")
- `sourceProject` - Source project (default: "openSUSE:Factory")
- `targetVersion` - Target version (default: "2.13")

### 3. Build Prompt (Code Node)
Constructs the AI prompt with actual values. This is necessary because the AI Agent node doesn't evaluate n8n expressions in the prompt text field.

### 4. AI Agent (LangChain)
Uses GPT-4o to orchestrate the upgrade process by calling osc-mcp tools in sequence.

### 5. osc-mcp Tools (MCP Client)
Connects to the osc-mcp server and exposes these tools to the AI:
- `branch_bundle` - Branch a package
- `list_source_files` - Get spec file content
- `edit_file` - Update spec file
- `commit` - Commit changes

## API Usage

### Trigger a Package Upgrade

```bash
curl -X POST "http://<n8n-host>:<port>/webhook/obs-upgrade" \
  -H "Content-Type: application/json" \
  -d '{
    "packageName": "hello",
    "sourceProject": "openSUSE:Factory",
    "targetVersion": "2.13"
  }'
```

### Response

On success:
```json
{
  "result": "### Summary\n\n- **Branch Project:** `home:spectro:branches:openSUSE:Factory`\n- **OBS URL:** [link](https://build.opensuse.org/package/show/home:spectro:branches:openSUSE:Factory/hello)\n- **Revision:** 2\n\nThe package `hello` has been successfully updated to version 2.13."
}
```

On error:
```json
{
  "result": "It seems there was an error with the API request..."
}
```

## Configuration

### osc-mcp Server

The osc-mcp server must be accessible from the n8n pods. For Kubernetes deployments:

1. Create a Service pointing to the osc-mcp host:
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

2. Configure the MCP Client Tool endpoint:
   - Endpoint URL: `http://osc-mcp.n8n.svc.cluster.local:8666/mcp`
   - Server Transport: `httpStreamable`
   - Authentication: `none`
   - Timeout: `180000` (3 minutes)

### OpenAI Credentials

The workflow requires OpenAI API credentials configured in n8n with access to `gpt-4o` model.

### OBS Credentials

The osc-mcp server must be configured with valid OBS credentials in `~/.config/osc/oscrc`:
```ini
[general]
apiurl=https://api.opensuse.org

[https://api.opensuse.org]
user=<username>
pass=<password>
credentials_mgr_class=osc.credentials.PlaintextConfigFileCredentialsManager
```

## Workflow ID

- **n8n Workflow ID:** `wvK2eTmn2swcqUBJ`
- **Name:** "OBS Package Upgrade - AI Agent with osc-mcp"

## Tool Call Sequence

The AI Agent executes tools in this order:

1. **branch_bundle**
   - Input: `project_name`, `bundle_name`
   - Output: `target_project`, `checkout_dir`

2. **list_source_files**
   - Input: `project_name` (from step 1), `package_name`, `local: true`
   - Output: Spec file content

3. **edit_file**
   - Input: `directory` (checkout_dir), `filename` (package.spec), `content` (modified spec)
   - Output: Success confirmation

4. **commit**
   - Input: `directory`, `message`
   - Output: Revision number

## Tested Packages

| Package | Source Project | Original Version | Target Version | Result |
|---------|---------------|------------------|----------------|--------|
| hello   | openSUSE:Factory | 2.12.2 | 2.13 | ✅ Success |
| bc      | openSUSE:Factory | 1.08.2 | 1.08.3 | ✅ Success |

## Limitations

1. **Spec file only** - Currently only updates the Version field in spec files
2. **No source download** - Does not download new source tarballs (run_services not called)
3. **No patch updates** - Does not handle patch file changes
4. **Single version field** - Assumes standard `Version:` field format

## Future Enhancements

- Add `run_services` call to download new source files
- Support for changelog entry generation
- Build validation before commit
- Submit request creation after commit
- Multi-package batch upgrades

## Troubleshooting

### "Could not connect to your MCP server"
- Verify osc-mcp is running: `systemctl status osc-mcp`
- Check Kubernetes Service/Endpoint configuration
- Test connectivity from n8n pod

### "404 Not Found" errors
- Verify package exists in source project
- Check OBS credentials are valid
- Ensure the user has permission to branch

### AI returns template syntax literally
- Ensure the Build Prompt (Code) node is in the workflow
- The AI Agent prompt text field doesn't evaluate n8n expressions directly
