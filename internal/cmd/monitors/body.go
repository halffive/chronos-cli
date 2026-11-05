package monitors

import (
	"io"

	chronos "chronos.sh/go"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"chronos.sh/cli/internal/input"
)

type bodyFlags struct {
	file        string
	name        string
	description string
	cron        string
	interval    string
	timezone    string
	grace       string
}

func (b *bodyFlags) register(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&b.file, "file", "", "Full request body as JSON, from a path or - for stdin")
	flags.StringVar(&b.name, "name", "", "Monitor name")
	flags.StringVar(&b.description, "description", "", "Monitor description")
	flags.StringVar(&b.cron, "cron", "", "Cron expression your job runs on, such as \"0 3 * * *\"")
	flags.StringVar(&b.interval, "interval", "", "Time between your job's runs, 1m to 365d, such as 1h or 1d")
	flags.StringVar(&b.timezone, "timezone", "", "IANA timezone the cron runs in, such as Europe/London")
	flags.StringVar(&b.grace, "grace", "", "How long past the expected ping to wait before alerting, 1m to 7d, such as 10m")
	cmd.MarkFlagsMutuallyExclusive("cron", "interval")
}

type body struct {
	Name         *string
	Description  *string
	Cron         *string
	Interval     *chronos.Interval
	Timezone     *string
	GraceSeconds *int
}

func (b *bodyFlags) read(flags *pflag.FlagSet, stdin io.Reader, params any) (body, error) {
	if b.file != "" {
		if err := input.DecodeFile(b.file, stdin, params); err != nil {
			return body{}, err
		}
	}
	return b.parse(flags)
}

func (b *bodyFlags) parse(flags *pflag.FlagSet) (body, error) {
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
		Cron:        str("cron", b.cron),
		Timezone:    str("timezone", b.timezone),
	}
	if changed("interval") {
		interval, err := input.ParseInterval(b.interval)
		if err != nil {
			return body{}, err
		}
		out.Interval = &interval
	}
	if changed("grace") {
		grace, err := input.ParseSeconds(b.grace)
		if err != nil {
			return body{}, err
		}
		out.GraceSeconds = &grace
	}
	return out, nil
}

func (b body) applyCreate(p *chronos.MonitorCreateParams) {
	if b.Name != nil {
		p.Name = *b.Name
	}
	set(&p.Description, b.Description)
	if b.Cron != nil {
		p.Cron, p.Interval = b.Cron, nil
	}
	if b.Interval != nil {
		p.Interval, p.Cron = b.Interval, nil
	}
	set(&p.Timezone, b.Timezone)
	set(&p.GraceSeconds, b.GraceSeconds)
}

func (b body) applyUpdate(p *chronos.MonitorUpdateParams) {
	set(&p.Name, b.Name)
	if b.Description != nil {
		p.Description = chronos.NewNullable(*b.Description)
	}
	if b.Cron != nil {
		p.Cron, p.Interval = b.Cron, nil
	}
	if b.Interval != nil {
		p.Interval, p.Cron = b.Interval, nil
	}
	set(&p.Timezone, b.Timezone)
	set(&p.GraceSeconds, b.GraceSeconds)
}

func set[T any](dst **T, v *T) {
	if v != nil {
		*dst = v
	}
}
