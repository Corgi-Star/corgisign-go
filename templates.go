package corgisign

import "context"

// TemplatesService accesses the template endpoints.
type TemplatesService struct {
	c *Client
}

// List returns the templates available to the API key, each including its
// recipient placeholders so you can build a role mapping for Envelopes.Create.
// Filter by team with WithTeamID (ignored for team-pinned keys).
func (s *TemplatesService) List(ctx context.Context, opts ...RequestOption) ([]Template, error) {
	var out struct {
		Templates []Template `json:"templates"`
	}
	if err := s.c.do(ctx, "GET", "/templates", nil, &out, opts); err != nil {
		return nil, err
	}
	return out.Templates, nil
}
