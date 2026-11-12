package knowledge

import (
	"context"
	"strings"
)

func (s *Store) Extracted(ctx context.Context, j Job, v Source, pages []Page) error {
	parts := []string{}
	for _, p := range pages {
		parts = append(parts, p.Text)
	}
	content := strings.Join(parts, "\n\n")
	hash := v.Checksum
	if v.Kind != "document" {
		hash = checksum([]byte(content))
	}
	_, e := s.DB.Exec(ctx, "UPDATE knowledge_sources SET content=$5,title=$6,language=$7,checksum=$8 WHERE organization_id=$1 AND assistant_id=$2 AND id=$3 AND revision=$4", j.OrganizationID, j.AssistantID, j.SourceID, j.Revision, content, v.Title, v.Language, hash)
	return e
}
