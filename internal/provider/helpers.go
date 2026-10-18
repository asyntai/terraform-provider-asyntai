package provider

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// clientFrom unpacks the client the provider handed to a resource or data
// source. ProviderData is nil during validation, before Configure has run.
func clientFrom(data interface{}, diags *diag.Diagnostics) *Client {
	if data == nil {
		return nil
	}
	client, ok := data.(*Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *Client, got %T.", data))
		return nil
	}
	return client
}

// sameText compares two texts the way the API stores them: it trims
// surrounding white space, so a heredoc's trailing newline is not drift.
func sameText(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

var httpURL = regexp.MustCompile(`^https?://`)
