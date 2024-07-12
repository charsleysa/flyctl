package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newSbom() *cobra.Command {
	const (
		usage = "sbom"
		short = "Generate an SBOM for a registry iamge"
		long  = "Genearte an SBOM for a registry image.\n" +
			"The image is selected by name, or the image of the app's first machine\n" +
			"is used unless interactive machine selection or machine ID is specified."
	)
	cmd := command.New(usage, short, long, runSbom,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.String{
			Name:        "image",
			Shorthand:   "i",
			Description: "Scan the repository image",
		},
		flag.String{
			Name:        "machine",
			Description: "Scan the image of the machine with the specified ID",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select which machine to scan the image of from a list.",
			Default:     false,
		},
	)

	return cmd
}

func runSbom(ctx context.Context) error {
	var (
		ios       = iostreams.FromContext(ctx)
		appName   = appconfig.NameFromContext(ctx)
		apiClient = flyutil.ClientFromContext(ctx)
	)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %w", err)
	}

	imgPath, err := argsGetImgPath(ctx, app)
	if err != nil {
		return err
	}

	token, err := makeScantronToken(ctx, app.Organization.ID, app.ID)
	if err != nil {
		return err
	}

	res, err := scantronSbomReq(ctx, imgPath, token)
	if err != nil {
		return err
	}
	defer res.Body.Close() // skipcq: GO-S2307

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed fetching SBOM (status code %d)", res.StatusCode)
	}

	if _, err := io.Copy(ios.Out, res.Body); err != nil {
		return fmt.Errorf("failed to read SBOM: %w", err)
	}
	return nil
}
