# osc-mcp Server Setup Playbook

This playbook guides you through setting up the osc-mcp MCP server for AI-assisted OBS package management.

## Prerequisites

- Go 1.21 or later
- OBS account with API access
- osc client installed and configured

## Step 1: Configure OBS Credentials

osc-mcp reads credentials from the standard osc configuration files.

### Option A: Plain text config (simplest)

Create or edit `~/.config/osc/oscrc`:

```ini
[general]
apiurl=https://api.opensuse.org

[https://api.opensuse.org]
user=<your-username>
pass=<your-password>
credentials_mgr_class=osc.credentials.PlaintextConfigFileCredentialsManager
```

### Option B: Keyring (recommended for security)

If you already use osc with keyring integration, osc-mcp will automatically read credentials from:
- D-Bus Secret Service (GNOME Keyring, KWallet)
- Kernel keyring (keyutils)

To set up keyring credentials with osc:
```bash
osc -A https://api.opensuse.org ls
# Enter username and password when prompted
# osc will store them in your keyring
```

## Step 2: Build osc-mcp

```bash
git clone https://github.com/openSUSE/osc-mcp.git
cd osc-mcp
go build .
```

## Step 3: Start the Server

### Basic startup

```bash
./osc-mcp --http localhost:8666 --workdir /tmp/mcp/osc-mcp/ --clean-workdir
```

### With verbose logging

```bash
./osc-mcp --http localhost:8666 --workdir /tmp/mcp/osc-mcp/ --clean-workdir -v
```

### With debug logging to file

```bash
./osc-mcp --http localhost:8666 --workdir /tmp/mcp/osc-mcp/ --clean-workdir -d --logfile /var/log/osc-mcp.log
```

### Production deployment with JSON logs

```bash
./osc-mcp --http 0.0.0.0:8666 --workdir /var/lib/osc-mcp/ -v --log-json --logfile /var/log/osc-mcp.json
```

## Step 4: Verify Installation

Test the server is running:

```bash
curl http://localhost:8666/mcp
```

You should see the MCP protocol response.

## Step 5: Configure Your AI Client

### gemini-cli

Add to `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "osc-mcp": {
      "httpUrl": "http://localhost:8666"
    }
  },
  "include-directories": ["/tmp/mcp/osc-mcp"]
}
```

### mcphost

Add to `~/.mcphost.yml`:

```yaml
mcpServers:
  osc-mcp:
    type: "remote"
    url: "http://localhost:8666"
```

### Claude Code

Add to your MCP configuration:

```json
{
  "mcpServers": {
    "osc-mcp": {
      "url": "http://localhost:8666/mcp"
    }
  }
}
```

### n8n AI Agent

See [n8n-integration.md](n8n-integration.md) for detailed setup instructions.

## Step 6: Systemd Service (Optional)

For persistent operation, create a systemd service:

```ini
# /etc/systemd/system/osc-mcp.service
[Unit]
Description=OBS MCP Server
After=network.target

[Service]
Type=simple
User=osc-mcp
ExecStart=/usr/local/bin/osc-mcp --http 0.0.0.0:8666 --workdir /var/lib/osc-mcp/ -v --log-json --logfile /var/log/osc-mcp.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable osc-mcp
sudo systemctl start osc-mcp
```

## Available Tools

Once running, the following tools are available to AI clients:

| Tool | Description |
|------|-------------|
| `search_bundle` | Search packages on OBS |
| `list_source_files` | List and read source files |
| `branch_bundle` | Branch a package for modification |
| `checkout_bundle` | Checkout a package locally |
| `edit_file` | Modify files in checked out packages |
| `build_bundle` | Build a package locally |
| `commit` | Commit changes to OBS |
| `get_build_log` | Retrieve build logs |
| `get_project_meta` | Get project metadata |
| `set_project_meta` | Update project metadata |

## Troubleshooting

### 403 Forbidden on commit

This is usually caused by stale authentication cookies. The server automatically clears the cookie cache before operations, but if issues persist:

```bash
rm -f ~/.local/state/osc/cookiejar
```

### Connection refused

Verify the server is running:

```bash
systemctl status osc-mcp
# or
pgrep -f osc-mcp
```

### Permission denied

Ensure your OBS user has permission to branch/modify packages in the target project.

### Missing credentials

Check that your oscrc is properly configured:

```bash
osc -A https://api.opensuse.org whoami
```
