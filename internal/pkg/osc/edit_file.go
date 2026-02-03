package osc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type EditFileParams struct {
	Directory string `json:"directory" jsonschema:"required,The checkout directory path where the file is located"`
	Filename  string `json:"filename" jsonschema:"required,The name of the file to edit"`
	Content   string `json:"content" jsonschema:"required,The new content to write to the file"`
}

type EditFileResult struct {
	Success  bool   `json:"success"`
	Path     string `json:"path"`
	Size     int    `json:"size"`
	Message  string `json:"message,omitempty"`
}

func (cred *OSCCredentials) EditFile(ctx context.Context, req *mcp.CallToolRequest, params EditFileParams) (*mcp.CallToolResult, any, error) {
	slog.Debug("mcp tool call: EditFile", "session", req.Session.ID(), "params", params)

	if params.Directory == "" {
		return nil, nil, fmt.Errorf("directory cannot be empty")
	}
	if params.Filename == "" {
		return nil, nil, fmt.Errorf("filename cannot be empty")
	}
	if params.Content == "" {
		return nil, nil, fmt.Errorf("content cannot be empty")
	}

	// Security check: ensure directory is under the temp dir
	absDir, err := filepath.Abs(params.Directory)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve directory path: %w", err)
	}

	absTempDir, err := filepath.Abs(cred.TempDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve temp dir path: %w", err)
	}

	if !strings.HasPrefix(absDir, absTempDir) {
		return nil, nil, fmt.Errorf("directory must be under the working directory %s", cred.TempDir)
	}

	// Security check: prevent path traversal in filename
	if strings.Contains(params.Filename, "..") || strings.Contains(params.Filename, "/") {
		return nil, nil, fmt.Errorf("filename cannot contain path separators or '..'")
	}

	filePath := filepath.Join(absDir, params.Filename)

	// Verify the file path is still under temp dir after joining
	if !strings.HasPrefix(filePath, absTempDir) {
		return nil, nil, fmt.Errorf("resulting file path is outside working directory")
	}

	// Check if directory exists
	if _, err := os.Stat(absDir); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("directory does not exist: %s", params.Directory)
	}

	// Write the file
	err = os.WriteFile(filePath, []byte(params.Content), 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	slog.Info("File written successfully", "path", filePath, "size", len(params.Content))

	return nil, EditFileResult{
		Success: true,
		Path:    filePath,
		Size:    len(params.Content),
		Message: fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.Filename),
	}, nil
}
