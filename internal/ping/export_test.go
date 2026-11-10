package ping

import "time"

func (c *Client) SetRetryDelay(d time.Duration) { c.retryDelay = d }
