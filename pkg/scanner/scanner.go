package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	// Register only the three backends described in the PRD.
	// Additional backends can be added here as needed.
	_ "github.com/rclone/rclone/backend/azureblob"
	_ "github.com/rclone/rclone/backend/local"
	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config/configfile"
	"github.com/rclone/rclone/fs/walk"
)

// FileMeta represents metadata for a scanned object.
type FileMeta struct {
	Path     string    `json:"Path"`
	Name     string    `json:"Name"`
	Size     int64     `json:"Size"`
	MimeType string    `json:"MimeType"`
	ModTime  time.Time `json:"ModTime"`
	IsDir    bool      `json:"IsDir"`
}

// Options configures a scan run.
type Options struct {
	// Anon uses anonymous (unauthenticated) access. Useful for public S3 buckets.
	Anon bool
	// Workers sets the number of listing threads (passed to rclone). 0 = rclone default.
	Workers int
	// Progress is an optional callback invoked after each batch of objects. It
	// receives the running total of files and bytes scanned so far.
	Progress func(files, bytes int64)
}

func init() {
	// Install a no-op config file handler so rclone works without ~/.config/rclone/rclone.conf.
	// Credentials are sourced from environment variables and inline remote parameters.
	configfile.Install()
}

// translateTarget converts common URI shorthand into rclone remote notation.
//
//	s3://bucket/prefix   → :s3,provider=AWS,env_auth=true:bucket/prefix
//	azure://container/p  → :azureblob:container/p
//	/local/path          → passed through (rclone local backend)
func translateTarget(target string, anon bool) string {
	switch {
	case strings.HasPrefix(target, "s3://"):
		suffix := strings.TrimPrefix(target, "s3://")
		if anon {
			return ":s3,provider=AWS,env_auth=false,anon=true:" + suffix
		}
		return ":s3,provider=AWS,env_auth=true:" + suffix

	case strings.HasPrefix(target, "azure://"):
		// Credentials via AZURE_STORAGE_ACCOUNT + AZURE_STORAGE_KEY
		// or AZURE_STORAGE_CONNECTION_STRING in the environment.
		return ":azureblob:" + strings.TrimPrefix(target, "azure://")
	}
	// Local paths and pre-formatted rclone remotes pass through unchanged.
	return target
}

// Scan streams file metadata from target and writes newline-delimited JSON to out.
// Memory usage is O(1) — objects are streamed from the rclone backend directly.
// No external rclone binary is required.
func Scan(target string, opts Options, out io.Writer) error {
	ctx := context.Background()

	rcloneTarget := translateTarget(target, opts.Anon)
	f, err := fs.NewFs(ctx, rcloneTarget)
	if err != nil {
		return fmt.Errorf("failed to initialise filesystem for %q: %w", target, err)
	}

	encoder := json.NewEncoder(out)
	var totalFiles, totalBytes int64

	err = walk.ListR(ctx, f, "", true, -1, walk.ListObjects, func(entries fs.DirEntries) error {
		for _, entry := range entries {
			obj, ok := entry.(fs.Object)
			if !ok {
				continue
			}

			name := filepath.Base(obj.Remote())

			// Detect MIME type — prefer backend-reported, fall back to extension.
			mimeType := ""
			if mt, ok := obj.(fs.MimeTyper); ok {
				mimeType = mt.MimeType(ctx)
			}
			if mimeType == "" {
				mimeType = mime.TypeByExtension(filepath.Ext(name))
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			meta := FileMeta{
				Path:     obj.Remote(),
				Name:     name,
				Size:     obj.Size(),
				MimeType: mimeType,
				ModTime:  obj.ModTime(ctx),
				IsDir:    false,
			}

			if err := encoder.Encode(meta); err != nil {
				return fmt.Errorf("failed to write metadata: %w", err)
			}

			totalFiles++
			totalBytes += meta.Size
		}

		if opts.Progress != nil {
			opts.Progress(totalFiles, totalBytes)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	return nil
}
