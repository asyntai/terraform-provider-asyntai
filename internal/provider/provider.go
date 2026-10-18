package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultBaseURL = "https://asyntai.com"

var _ provider.Provider = &asyntaiProvider{}

type asyntaiProvider struct {
	version string
}

type providerModel struct {
	APIKey  types.String `tfsdk:"api_key"`
	BaseURL types.String `tfsdk:"base_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &asyntaiProvider{version: version}
	}
}

func (p *asyntaiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "asyntai"
	resp.Version = p.version
}

func (p *asyntaiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage Asyntai AI chatbots as code: websites (AI agents), widget settings, AI instructions, " +
			"knowledge base items and webhooks. Uses the [Asyntai REST API](https://asyntai.com/documentation/api/), " +
			"which needs a paid Asyntai plan.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Asyntai API key, from the API Settings page (https://asyntai.com/settings/api/). " +
					"Can also be set with the `ASYNTAI_API_KEY` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "API address. Defaults to `https://asyntai.com`. " +
					"Can also be set with the `ASYNTAI_BASE_URL` environment variable.",
				Optional: true,
			},
		},
	}
}

func (p *asyntaiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.APIKey.IsUnknown() || config.BaseURL.IsUnknown() {
		// Values come from another resource that is not applied yet.
		return
	}

	apiKey := os.Getenv("ASYNTAI_API_KEY")
	if !config.APIKey.IsNull() {
		apiKey = config.APIKey.ValueString()
	}
	baseURL := os.Getenv("ASYNTAI_BASE_URL")
	if !config.BaseURL.IsNull() {
		baseURL = config.BaseURL.ValueString()
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing Asyntai API key",
			"Set api_key in the provider block or the ASYNTAI_API_KEY environment variable. "+
				"You find your key on the API Settings page: https://asyntai.com/settings/api/")
		return
	}

	client := NewClient(baseURL, apiKey, p.version)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *asyntaiProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWebsiteResource,
		NewWebsiteSettingsResource,
		NewInstructionsResource,
		NewKnowledgeTextResource,
		NewKnowledgeURLResource,
		NewWebhookResource,
	}
}

func (p *asyntaiProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewWebsitesDataSource,
		NewWebsiteDataSource,
	}
}
