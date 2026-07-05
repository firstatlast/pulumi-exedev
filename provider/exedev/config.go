package exedev

import (
	"context"
	"fmt"

	"github.com/pulumi/pulumi-go-provider/infer"
)

type Config struct {
	Token    string `pulumi:"token,optional" provider:"secret"`
	Endpoint string `pulumi:"endpoint,optional"`

	client *Client
}

var (
	_ infer.Annotated       = (*Config)(nil)
	_ infer.CustomConfigure = (*Config)(nil)
)

func (c *Config) Annotate(a infer.Annotator) {
	a.Describe(&c.Token, "exe.dev bearer API token. Generate with `ssh exe.dev ssh-key generate-api-key --exp=30d`.")
	a.Describe(&c.Endpoint, "Override for the exe.dev exec endpoint.")
	a.SetDefault(&c.Token, "", "EXEDEV_TOKEN")
	a.SetDefault(&c.Endpoint, DefaultEndpoint, "EXEDEV_ENDPOINT")
}

func (c *Config) Configure(_ context.Context) error {
	if c.Token == "" {
		return fmt.Errorf("exe.dev token is required; set `exedev:token` or the EXEDEV_TOKEN environment variable")
	}
	c.client = NewClient(c.Token, c.Endpoint)
	return nil
}

func clientFromContext(ctx context.Context) *Client {
	return infer.GetConfig[Config](ctx).client
}
