package main

import (
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/winebarrel/tfunfold"
)

var version string

func init() {
	log.SetFlags(0)
}

type options struct {
	Dir     string `arg:"" optional:"" default:"." help:"Directory containing *.tf files (default: \".\")."`
	State   string `short:"s" help:"Path to a terraform.tfstate file. When omitted, 'terraform state pull' is invoked in <dir>."`
	InPlace bool   `short:"i" help:"Write changes back to files instead of stdout."`
	Version kong.VersionFlag
}

func parseArgs() *options {
	opts := &options{}
	parser := kong.Must(opts,
		kong.Name("tfunfold"),
		kong.Description("Expand Terraform for_each / count into individual resources or modules with moved blocks."),
		kong.Vars{"version": version},
	)
	parser.Model.HelpFlag.Help = "Show help."
	if _, err := parser.Parse(os.Args[1:]); err != nil {
		parser.FatalIfErrorf(err)
	}
	return opts
}

func main() {
	opts := parseArgs()
	u := tfunfold.NewUnfolder(opts.Dir)
	if opts.State != "" {
		u.StatePath = opts.State
	}
	if err := u.Unfold(opts.InPlace); err != nil {
		log.Fatalf("error: %v", err)
	}
}
