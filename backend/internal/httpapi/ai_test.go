package httpapi

import (
	"context"
	"site-assistant/internal/knowledge"
)

type previewTestRetriever struct{}

func (previewTestRetriever) Search(context.Context, string, string, string, knowledge.SearchInput) ([]knowledge.Hit, error) {
	return []knowledge.Hit{}, nil
}
