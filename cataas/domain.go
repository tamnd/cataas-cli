package cataas

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes cataas as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/cataas-cli/cataas"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// cataas:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone cataas binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the cataas driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "cataas",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "cataas",
			Short:  "A command line for CATAAS (Cat As A Service).",
			Long: `A command line for cataas.com - Cat As A Service.

cataas reads public cat data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it.`,
			Site: Host,
			Repo: "https://github.com/tamnd/cataas-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// random: get a random cat, optionally filtered by tag.
	kit.Handle(app, kit.OpMeta{Name: "random", Group: "read", Single: true,
		Summary: "Get a random cat"}, getRandomCat)

	// list: list cats, optionally filtered by tag.
	kit.Handle(app, kit.OpMeta{Name: "list", Group: "read", List: true,
		Summary: "List cats"}, listCats)

	// tags: list all available tags.
	kit.Handle(app, kit.OpMeta{Name: "tags", Group: "read", List: true,
		Summary: "List available tags"}, listTags)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type randomInput struct {
	Tag    string  `kit:"flag" help:"filter by tag"`
	Client *Client `kit:"inject"`
}

type listInput struct {
	Tag    string  `kit:"flag" help:"filter by tag"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type tagsInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getRandomCat(ctx context.Context, in randomInput, emit func(*Cat) error) error {
	cat, err := in.Client.RandomCat(ctx, in.Tag)
	if err != nil {
		return mapErr(err)
	}
	return emit(cat)
}

func listCats(ctx context.Context, in listInput, emit func(*Cat) error) error {
	cats, err := in.Client.ListCats(ctx, in.Tag, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, cat := range cats {
		if err := emit(cat); err != nil {
			return err
		}
	}
	return nil
}

func listTags(ctx context.Context, in tagsInput, emit func(*Tag) error) error {
	tags, err := in.Client.ListTags(ctx, in.Limit)
	if err != nil {
		return mapErr(err)
	}
	for _, tag := range tags {
		if err := emit(tag); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI driver pure string functions ---

// Classify turns a cat id or URL into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("empty cataas reference")
	}
	return "cat", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "cat" {
		return "", errs.Usage("cataas has no resource type %q", uriType)
	}
	return BaseURL + "/cat/" + id, nil
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
