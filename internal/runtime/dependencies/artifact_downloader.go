package dependency

import (
	"context"
	"io"
)

// ArtifactDownloader is the narrow artifact acquisition port shared by the
// managed dependency capabilities.
type ArtifactDownloader interface {
	Download(context.Context, string, io.Writer) error
}
