package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/asyntai/terraform-provider-asyntai/internal/provider"
)

// Set by GoReleaser at build time.
var (
	version = "dev"
	commit  = ""
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name asyntai

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/asyntai/asyntai",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
