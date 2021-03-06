package cmd

import (
	"fmt"
	"github.com/gonuts/commander"
	"github.com/gonuts/flag"
	"github.com/smira/aptly/debian"
	"github.com/smira/aptly/utils"
	"strings"
)

func aptlyPublishSnapshot(cmd *commander.Command, args []string) error {
	var err error
	if len(args) < 1 || len(args) > 2 {
		cmd.Usage()
		return err
	}

	name := args[0]

	var prefix string
	if len(args) == 2 {
		prefix = args[1]
	} else {
		prefix = ""
	}

	publishedCollecton := debian.NewPublishedRepoCollection(context.database)

	snapshotCollection := debian.NewSnapshotCollection(context.database)
	snapshot, err := snapshotCollection.ByName(name)
	if err != nil {
		return fmt.Errorf("unable to publish: %s", err)
	}

	err = snapshotCollection.LoadComplete(snapshot)
	if err != nil {
		return fmt.Errorf("unable to publish: %s", err)
	}

	var sourceRepo *debian.RemoteRepo

	if snapshot.SourceKind == "repo" && len(snapshot.SourceIDs) == 1 {
		repoCollection := debian.NewRemoteRepoCollection(context.database)

		sourceRepo, _ = repoCollection.ByUUID(snapshot.SourceIDs[0])
	}

	component := cmd.Flag.Lookup("component").Value.String()
	if component == "" {
		if sourceRepo != nil && len(sourceRepo.Components) == 1 {
			component = sourceRepo.Components[0]
		} else {
			component = "main"
		}
	}

	distribution := cmd.Flag.Lookup("distribution").Value.String()
	if distribution == "" {
		if sourceRepo != nil {
			distribution = sourceRepo.Distribution
		}

		if distribution == "" {
			return fmt.Errorf("unable to guess distribution name, please specify explicitly")
		}
	}

	published, err := debian.NewPublishedRepo(prefix, distribution, component, context.architecturesList, snapshot)
	if err != nil {
		return fmt.Errorf("unable to publish: %s", err)
	}

	duplicate := publishedCollecton.CheckDuplicate(published)
	if duplicate != nil {
		publishedCollecton.LoadComplete(duplicate, snapshotCollection)
		return fmt.Errorf("prefix/distribution already used by another published repo: %s", duplicate)
	}

	signer, err := getSigner(cmd)
	if err != nil {
		return fmt.Errorf("unable to initialize GPG signer: %s", err)
	}

	packageCollection := debian.NewPackageCollection(context.database)
	err = published.Publish(context.packagePool, context.publishedStorage, packageCollection, signer, context.progress)
	if err != nil {
		return fmt.Errorf("unable to publish: %s", err)
	}

	err = publishedCollecton.Add(published)
	if err != nil {
		return fmt.Errorf("unable to save to DB: %s", err)
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	context.progress.Printf("\nSnapshot %s has been successfully published.\nPlease setup your webserver to serve directory '%s' with autoindexing.\n",
		snapshot.Name, context.publishedStorage.PublicPath())
	context.progress.Printf("Now you can add following line to apt sources:\n")
	context.progress.Printf("  deb http://your-server/%s %s %s\n", prefix, distribution, component)
	if utils.StrSliceHasItem(published.Architectures, "source") {
		context.progress.Printf("  deb-src http://your-server/%s %s %s\n", prefix, distribution, component)
	}
	context.progress.Printf("Don't forget to add your GPG key to apt with apt-key.\n")
	context.progress.Printf("\nYou can also use `aptly serve` to publish your repositories over HTTP quickly.\n")

	return err
}

func makeCmdPublishSnapshot() *commander.Command {
	cmd := &commander.Command{
		Run:       aptlyPublishSnapshot,
		UsageLine: "snapshot <name> [<prefix>]",
		Short:     "publish snapshot",
		Long: `
Command publish publishes snapshot as Debian repository ready to be consumed
by apt tools. Published repostiories appear under rootDir/public directory.
Valid GPG key is required for publishing.

Example:

    $ aptly publish snapshot wheezy-main
`,
		Flag: *flag.NewFlagSet("aptly-publish-snapshot", flag.ExitOnError),
	}
	cmd.Flag.String("distribution", "", "distribution name to publish")
	cmd.Flag.String("component", "", "component name to publish")
	cmd.Flag.String("gpg-key", "", "GPG key ID to use when signing the release")
	cmd.Flag.String("keyring", "", "GPG keyring to use (instead of default)")
	cmd.Flag.String("secret-keyring", "", "GPG secret keyring to use (instead of default)")
	cmd.Flag.Bool("skip-signing", false, "don't sign Release files with GPG")

	return cmd
}
