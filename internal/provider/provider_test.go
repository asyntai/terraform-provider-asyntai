package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Acceptance tests run real Terraform against an Asyntai API. Point them at
// the local rig (rig/serve.py), never at a production account:
//
//	TF_ACC=1 ASYNTAI_BASE_URL=http://127.0.0.1:8015 ASYNTAI_API_KEY=... go test ./... -v
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"asyntai": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	for _, name := range []string{"ASYNTAI_API_KEY", "ASYNTAI_BASE_URL"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s must be set for acceptance tests", name)
		}
	}
}

func testClient() *Client {
	return NewClient(os.Getenv("ASYNTAI_BASE_URL"), os.Getenv("ASYNTAI_API_KEY"), "test")
}

// apiGet reads an API path into out, for checks against the API itself.
func apiGet(path string, out interface{}) error {
	return testClient().Do(context.Background(), "GET", path, nil, out)
}

// attr reads one attribute of a resource from the Terraform state.
func attr(s *terraform.State, name, key string) (string, error) {
	rs, ok := s.RootModule().Resources[name]
	if !ok {
		return "", fmt.Errorf("%s not in state", name)
	}
	return rs.Primary.Attributes[key], nil
}
