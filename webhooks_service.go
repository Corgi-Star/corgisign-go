package corgisign

import "context"

// WebhooksService registers outbound webhook subscriptions. To verify the
// signature on an inbound webhook delivery, use the sibling package
// github.com/Corgi-Star/corgisign-go/webhooks.
type WebhooksService struct {
	c *Client
}

// Register creates an outbound webhook. The returned Webhook.Secret (the HMAC
// signing secret) is returned exactly once; persist it to verify deliveries.
func (s *WebhooksService) Register(ctx context.Context, req RegisterWebhook, opts ...RequestOption) (*Webhook, error) {
	var wh Webhook
	if err := s.c.do(ctx, "POST", "/webhooks", req, &wh, opts); err != nil {
		return nil, err
	}
	return &wh, nil
}
