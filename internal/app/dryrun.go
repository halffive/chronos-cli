package app

import (
	"bytes"
	"encoding/json"
	"fmt"

	"chronos.sh/cli/internal/output"
	"chronos.sh/cli/internal/transport"
)

// DryRunView shows a captured request as {"dry_run": {method, url, body}}.
// It masks push header values and a password in the push URL. The URL's
// username and query string stay visible.
func DryRunView(c *transport.Captured) output.View {
	shown := *c
	shown.Body = maskPushSecrets(c.Body)
	value := map[string]transport.Captured{"dry_run": shown}
	return output.ViewOf(value, func(p *output.Printer) error {
		fmt.Fprintf(p.Out, "Dry run, nothing sent:\n%s %s\n", shown.Method, output.Clean(shown.URL))
		if shown.Body == nil {
			return nil
		}
		var body bytes.Buffer
		if err := json.Indent(&body, shown.Body, "", "  "); err != nil {
			return err
		}
		_, err := fmt.Fprintln(p.Out, output.Clean(body.String()))
		return err
	})
}

func maskPushSecrets(body json.RawMessage) json.RawMessage {
	if body == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var request map[string]any
	if err := dec.Decode(&request); err != nil {
		return body
	}
	delivery, _ := request["delivery"].(map[string]any)
	httpDelivery, _ := delivery["http"].(map[string]any)
	headers, _ := httpDelivery["headers"].(map[string]any)
	pushURL, _ := httpDelivery["url"].(string)
	redacted := output.RedactedURL(pushURL)
	if len(headers) == 0 && redacted == pushURL {
		return body
	}
	for name := range headers {
		headers[name] = output.Masked
	}
	if redacted != pushURL {
		httpDelivery["url"] = redacted
	}
	masked, err := json.Marshal(request)
	if err != nil {
		return body
	}
	return masked
}
