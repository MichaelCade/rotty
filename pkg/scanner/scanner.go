package scanner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// FileMeta represents metadata for a scanned object from rclone
type FileMeta struct {
	Path    string    `json:"Path"`
	Name    string    `json:"Name"`
	Size    int64     `json:"Size"`
	MimeType string   `json:"MimeType"`
	ModTime time.Time `json:"ModTime"`
	IsDir   bool      `json:"IsDir"`
}

// CheckDeps ensures rclone is installed
func CheckDeps() error {
	_, err := exec.LookPath("rclone")
	if err != nil {
		return fmt.Errorf("rclone is required but not found in PATH")
	}
	return nil
}

// Scan initiates an rclone scan of the target, and writes newline-delimited JSON metadata
// about each file to the output stream. Memory is O(1) as it streams.
func Scan(target string, anon bool, out io.Writer) error {
	if err := CheckDeps(); err != nil {
		return err
	}

	// Auto-translate s3:// for out-of-the-box support without config
	if strings.HasPrefix(target, "s3://") {
		if anon {
			target = strings.Replace(target, "s3://", ":s3,provider=AWS,env_auth=false,anon=true:", 1)
		} else {
			target = strings.Replace(target, "s3://", ":s3,provider=AWS,env_auth=true:", 1)
		}
	}

	cmd := exec.Command("rclone", "lsjson", "-R", target)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("could not create stdout pipe: %w", err)
	}
	// Pipe stderr to os.Stderr so the user sees rclone errors directly
	cmd.Stderr = os.Stderr 

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start rclone: %w", err)
	}

	decoder := json.NewDecoder(stdout)
	encoder := json.NewEncoder(out)

	// Expect an array opening bracket
	t, err := decoder.Token()
	if err != nil {
		// rclone might have output nothing if directory is empty or missing, or not JSON.
		return fmt.Errorf("failed to read opening token from rclone output: %w", err)
	}
	if delim, ok := t.(json.Delim); !ok || delim != '[' {
		return fmt.Errorf("expected JSON array opening bracket, got %v", t)
	}

	for decoder.More() {
		var meta FileMeta
		if err := decoder.Decode(&meta); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode array element: %w", err)
		}
		
		// Write to output stream as NDJSON
		if err := encoder.Encode(meta); err != nil {
			return fmt.Errorf("failed to write to output: %w", err)
		}
	}

	// Consume the closing bracket
	if _, err := decoder.Token(); err != nil && err != io.EOF {
		return fmt.Errorf("failed to read closing token: %w", err)
	}

	err = cmd.Wait()
	if err != nil {
		return fmt.Errorf("rclone exited with error: %w", err)
	}
	return nil
}
