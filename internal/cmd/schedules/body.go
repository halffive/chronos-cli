package schedules

import (
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/exit"
	"chronos.sh/cli/internal/input"
)

type bodyFlags struct {
	file        string
	name        string
	description string
	handler     string
	cron        string
	interval    string
	timezone    string
	runs        int
	stopAt      string
	timeout     string
	maxRetries  int
	payload     string
	pushURL     string
	pushMethod  string
	pushHeaders []string
}

func (b *bodyFlags) register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&b.file, "file", "", "Full request body as JSON, from a path or - for stdin")
	flags.StringVar(&b.name, "name", "", "Schedule name")
	flags.StringVar(&b.description, "description", "", "Schedule description")
	flags.StringVar(&b.handler, "handler", "", "Handler name that workers claim jobs by; required unless jobs are pushed")
	flags.StringVar(&b.cron, "cron", "", "Cron expression, such as \"0 3 * * *\"")
	flags.StringVar(&b.interval, "interval", "", "Time between runs, such as 30s, 5m or 1d")
	flags.StringVar(&b.timezone, "timezone", "", "IANA timezone the cron runs in, such as Europe/London")
	flags.IntVar(&b.runs, "runs", 0, "Stop after this many runs")
	flags.StringVar(&b.stopAt, "stop-at", "", "Stop creating jobs after this RFC 3339 time")
	flags.StringVar(&b.timeout, "timeout", "", "Handler timeout, such as 30s or 5m")
	flags.IntVar(&b.maxRetries, "max-retries", 0, "Retries after a failed attempt")
	flags.StringVar(&b.payload, "payload", "", "JSON object passed to the handler with every job: inline, @path, or @- for stdin")
	flags.StringVar(&b.pushURL, "push-url", "", "Push jobs to this URL instead of waiting for a worker")
	flags.StringVar(&b.pushMethod, "push-method", "", "HTTP method for push delivery (default POST)")
	flags.StringArrayVar(&b.pushHeaders, "push-header", nil, "Header sent with each push, as 'Name: value' (repeatable)")
	cmd.MarkFlagsMutuallyExclusive("cron", "interval")
}

type body struct {
	Name        *string
	Description *string
	Handler     *string
	Cron        *string
	Interval    *chronos.Interval
	Timezone    *string
	Runs        *int
	StopAt      *time.Time
	Timeout     *int
	MaxRetries  *int
	Payload     map[string]any
	Push        *push
}

type push struct {
	URL     *string
	Method  *string
	Headers map[string]string
}

func (b *bodyFlags) read(flags *pflag.FlagSet, stdin io.Reader, params any) (body, error) {
	if b.file == "-" && b.payload == "@-" {
		return body{}, exit.Usagef("--file - and --payload @- both read stdin. Put the payload in the file instead.")
	}
	if b.file != "" {
		if err := input.DecodeFile(b.file, stdin, params); err != nil {
			return body{}, err
		}
	}
	return b.parse(flags, stdin)
}

func (b *bodyFlags) parse(flags *pflag.FlagSet, stdin io.Reader) (body, error) {
	changed := flags.Changed
	str := func(name, value string) *string {
		if !changed(name) {
			return nil
		}
		return new(value)
	}
	out := body{
		Name:        str("name", b.name),
		Description: str("description", b.description),
		Handler:     str("handler", b.handler),
		Cron:        str("cron", b.cron),
		Timezone:    str("timezone", b.timezone),
	}
	if changed("runs") {
		out.Runs = new(b.runs)
	}
	if changed("max-retries") {
		out.MaxRetries = new(b.maxRetries)
	}
	if changed("interval") {
		interval, err := input.ParseInterval(b.interval)
		if err != nil {
			return body{}, err
		}
		out.Interval = &interval
	}
	if changed("timeout") {
		timeout, err := input.ParseSeconds(b.timeout)
		if err != nil {
			return body{}, err
		}
		out.Timeout = &timeout
	}
	if changed("stop-at") {
		stopAt, err := input.ParseTime(b.stopAt)
		if err != nil {
			return body{}, err
		}
		out.StopAt = &stopAt
	}
	if changed("payload") {
		payload, err := input.ReadPayload(b.payload, stdin)
		if err != nil {
			return body{}, err
		}
		out.Payload = payload
	}
	p, err := b.parsePush(flags)
	if err != nil {
		return body{}, err
	}
	out.Push = p
	return out, nil
}

func (b *bodyFlags) parsePush(flags *pflag.FlagSet) (*push, error) {
	changed := flags.Changed
	if !changed("push-url") && !changed("push-method") && !changed("push-header") {
		return nil, nil
	}
	p := &push{Headers: map[string]string{}}
	if changed("push-url") {
		p.URL = new(b.pushURL)
	}
	if changed("push-method") {
		p.Method = new(b.pushMethod)
	}
	for _, header := range b.pushHeaders {
		name, value, err := input.ParsePushHeader(header)
		if err != nil {
			return nil, err
		}
		maps.DeleteFunc(p.Headers, func(k, _ string) bool { return strings.EqualFold(k, name) })
		p.Headers[name] = value
	}
	return p, nil
}

func (p *push) delivery(current *chronos.Delivery) *chronos.Delivery {
	if p == nil {
		return current
	}
	var h chronos.HTTPDelivery
	if current != nil && current.HTTP != nil {
		h = *current.HTTP
	}
	if p.URL != nil {
		h.URL = *p.URL
	}
	if p.Method != nil {
		h.Method = *p.Method
	}
	if h.Method == "" {
		h.Method = http.MethodPost
	}
	if len(p.Headers) > 0 {
		h.Headers = maps.Clone(h.Headers)
		if h.Headers == nil {
			h.Headers = map[string]string{}
		}
		for name, value := range p.Headers {
			maps.DeleteFunc(h.Headers, func(k, _ string) bool { return strings.EqualFold(k, name) })
			h.Headers[name] = value
		}
	}
	return &chronos.Delivery{Type: chronos.DeliveryTypePush, HTTP: &h}
}

func (b body) applyCreate(p *chronos.ScheduleCreateParams) {
	if b.Name != nil {
		p.Name = *b.Name
	}
	set(&p.Description, b.Description)
	set(&p.Handler, b.Handler)
	if b.Cron != nil {
		p.Cron, p.Interval = b.Cron, nil
	}
	if b.Interval != nil {
		p.Interval, p.Cron = b.Interval, nil
	}
	set(&p.Timezone, b.Timezone)
	set(&p.Runs, b.Runs)
	set(&p.StopAt, b.StopAt)
	set(&p.Timeout, b.Timeout)
	set(&p.MaxRetries, b.MaxRetries)
	if b.Payload != nil {
		p.Payload = b.Payload
	}
	p.Delivery = b.Push.delivery(p.Delivery)
}

func (b body) applyUpdate(p *chronos.ScheduleUpdateParams) {
	set(&p.Name, b.Name)
	setNullable(&p.Description, b.Description)
	setNullable(&p.Handler, b.Handler)
	if b.Cron != nil {
		p.Cron, p.Interval = b.Cron, nil
	}
	if b.Interval != nil {
		p.Interval, p.Cron = b.Interval, nil
	}
	set(&p.Timezone, b.Timezone)
	setNullable(&p.Runs, b.Runs)
	setNullable(&p.StopAt, b.StopAt)
	set(&p.Timeout, b.Timeout)
	set(&p.MaxRetries, b.MaxRetries)
	if b.Payload != nil {
		p.Payload = chronos.NewNullable(b.Payload)
	}
	p.Delivery = b.Push.delivery(p.Delivery)
}

func set[T any](dst **T, v *T) {
	if v != nil {
		*dst = v
	}
}

func setNullable[T any](dst *chronos.Nullable[T], v *T) {
	if v != nil {
		*dst = chronos.NewNullable(*v)
	}
}
