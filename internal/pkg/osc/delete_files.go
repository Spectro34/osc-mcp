package osc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeleteFilesParam struct {
	Directory string   `json:"directory" jsonschema:"The directory containing the files to delete"`
	Patterns  []string `json:"patterns" jsonschema:"File patterns to delete (e.g., '*.tar.gz', '*.obscpio'). Supports glob patterns."`
}

type DeleteFilesResult struct {
	Success      bool     `json:"success"`
	DeletedFiles []string `json:"deleted_files,omitempty"`
	Error        string   `json:"error,omitempty"`
}

func (cred *OSCCredentials) DeleteFiles(ctx context.Context, req *mcp.CallToolRequest, params DeleteFilesParam) (*mcp.CallToolResult, DeleteFilesResult, error) {
	slog.Debug("mcp tool call: DeleteFiles", "session", req.Session.ID(), "params", params)

	if params.Directory == "" {
		return nil, DeleteFilesResult{Success: false, Error: "directory must be specified"}, nil
	}
	if len(params.Patterns) == 0 {
		return nil, DeleteFilesResult{Success: false, Error: "at least one pattern must be specified"}, nil
	}

	// Ensure directory is within the allowed temp directory
	absDir, err := filepath.Abs(params.Directory)
	if err != nil {
		return nil, DeleteFilesResult{Success: false, Error: fmt.Sprintf("invalid directory: %v", err)}, nil
	}

	absTempDir, err := filepath.Abs(cred.TempDir)
	if err != nil {
		return nil, DeleteFilesResult{Success: false, Error: fmt.Sprintf("invalid temp directory: %v", err)}, nil
	}

	// Security check: ensure we're operating within the temp directory
	relPath, err := filepath.Rel(absTempDir, absDir)
	if err != nil || len(relPath) >= 2 && relPath[:2] == ".." {
		return nil, DeleteFilesResult{Success: false, Error: "directory must be within the osc-mcp work directory"}, nil
	}

	// Prepare osc config
	cmdlineCfg := []string{"osc"}
	configFile, err := cred.writeTempOscConfig()
	if err != nil {
		slog.Warn("failed to write osc config", "error", err)
	} else {
		defer os.Remove(configFile)
		cmdlineCfg = append(cmdlineCfg, "--config", configFile)
	}

	var deletedFiles []string

	for _, pattern := range params.Patterns {
		// Match files in the directory
		matches, err := filepath.Glob(filepath.Join(absDir, pattern))
		if err != nil {
			slog.Warn("invalid glob pattern", "pattern", pattern, "error", err)
			continue
		}

		for _, match := range matches {
			// Skip directories
			info, err := os.Stat(match)
			if err != nil {
				slog.Warn("failed to stat file", "file", match, "error", err)
				continue
			}
			if info.IsDir() {
				continue
			}

			fileName := filepath.Base(match)

			// Run osc rm to mark file for removal in osc
			oscRmCmd := append(cmdlineCfg, "rm", "-f", fileName)
			cmd := exec.CommandContext(ctx, oscRmCmd[0], oscRmCmd[1:]...)
			cmd.Dir = absDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				slog.Warn("osc rm failed, trying local delete", "file", fileName, "error", err, "output", string(output))
				// Fall back to local delete if osc rm fails
				if err := os.Remove(match); err != nil {
					slog.Warn("failed to delete file", "file", match, "error", err)
					continue
				}
			}

			deletedFiles = append(deletedFiles, fileName)
			slog.Info("removed file", "file", match)
		}
	}

	return nil, DeleteFilesResult{
		Success:      true,
		DeletedFiles: deletedFiles,
	}, nil
}
