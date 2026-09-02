package audit

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"
)

type listTagService struct {
	distribution.TagService
}

func (listTagService) List(ctx context.Context, limit int, last string) ([]string, error) {
	return []string{"latest"}, io.EOF
}

func TestTagServiceListEOFLogsSuccess(t *testing.T) {
	logger := NewLogger(context.Background())
	var buf bytes.Buffer
	logger.logger.Out = &buf
	ctx := WithLogger(context.Background(), logger)

	tags, err := NewTagService(ctx, listTagService{}).List(ctx, -1, "")
	if err != io.EOF {
		t.Fatalf("expected io.EOF to be passed through, got %v", err)
	}
	if len(tags) != 1 || tags[0] != "latest" {
		t.Fatalf("unexpected tags: %v", tags)
	}

	out := buf.String()
	if !strings.Contains(out, AuditStatusEntry+"="+OpStatusOK) || strings.Contains(out, AuditStatusEntry+"="+OpStatusError) {
		t.Fatalf("expected a success audit entry, got:\n%s", out)
	}
}
