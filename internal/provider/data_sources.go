package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---- asyntai_websites -------------------------------------------------------

var _ datasource.DataSourceWithConfigure = &websitesDataSource{}

func NewWebsitesDataSource() datasource.DataSource { return &websitesDataSource{} }

type websitesDataSource struct{ client *Client }

type websitesItem struct {
	ID        types.String `tfsdk:"id"`
	Domain    types.String `tfsdk:"domain"`
	IsPrimary types.Bool   `tfsdk:"is_primary"`
}

type websitesModel struct {
	Websites []websitesItem `tfsdk:"websites"`
}

func (d *websitesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_websites"
}

func (d *websitesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All websites (AI agents) in the Asyntai account.",
		Attributes: map[string]schema.Attribute{
			"websites": schema.ListNestedAttribute{
				MarkdownDescription: "The websites.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Website id."},
						"domain":     schema.StringAttribute{Computed: true, MarkdownDescription: "Domain."},
						"is_primary": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the primary website."},
					},
				},
			},
		},
	}
}

func (d *websitesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *websitesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var out struct {
		Websites []struct {
			ID        int64  `json:"id"`
			Domain    string `json:"domain"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"websites"`
	}
	if err := d.client.Do(ctx, "GET", "/api/v1/websites/", nil, &out); err != nil {
		resp.Diagnostics.AddError("Could not list the websites", err.Error())
		return
	}
	state := websitesModel{Websites: []websitesItem{}}
	for _, w := range out.Websites {
		state.Websites = append(state.Websites, websitesItem{
			ID:        types.StringValue(strconv.FormatInt(w.ID, 10)),
			Domain:    types.StringValue(w.Domain),
			IsPrimary: types.BoolValue(w.IsPrimary),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---- asyntai_website --------------------------------------------------------

var _ datasource.DataSourceWithConfigure = &websiteDataSource{}

func NewWebsiteDataSource() datasource.DataSource { return &websiteDataSource{} }

type websiteDataSource struct{ client *Client }

type websiteDataModel struct {
	ID             types.String `tfsdk:"id"`
	Domain         types.String `tfsdk:"domain"`
	Name           types.String `tfsdk:"name"`
	WidgetID       types.String `tfsdk:"widget_id"`
	IsPrimary      types.Bool   `tfsdk:"is_primary"`
	Ready          types.Bool   `tfsdk:"ready"`
	KnowledgeItems types.Int64  `tfsdk:"knowledge_items"`
	EmbedCode      types.String `tfsdk:"embed_code"`
}

func (d *websiteDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website"
}

func (d *websiteDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One website (AI agent): its widget id, embed code and whether its setup jobs are done.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Required: true, MarkdownDescription: "Website id."},
			"domain":    schema.StringAttribute{Computed: true, MarkdownDescription: "Domain."},
			"name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Display name."},
			"widget_id": schema.StringAttribute{Computed: true, MarkdownDescription: "Widget id."},
			"is_primary": schema.BoolAttribute{Computed: true,
				MarkdownDescription: "Whether this is the primary website."},
			"ready": schema.BoolAttribute{Computed: true,
				MarkdownDescription: "`true` when no site reading or instruction writing is still running."},
			"knowledge_items": schema.Int64Attribute{Computed: true,
				MarkdownDescription: "Number of knowledge base items."},
			"embed_code": schema.StringAttribute{Computed: true,
				MarkdownDescription: "The `<script>` snippet to put on the site."},
		},
	}
}

func (d *websiteDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (d *websiteDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg websiteDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var out struct {
		Website struct {
			ID             int64   `json:"id"`
			Domain         string  `json:"domain"`
			Name           *string `json:"name"`
			WidgetID       string  `json:"widget_id"`
			IsPrimary      bool    `json:"is_primary"`
			Ready          bool    `json:"ready"`
			KnowledgeItems int64   `json:"knowledge_items"`
		} `json:"website"`
		EmbedCode string `json:"embed_code"`
	}
	if err := d.client.Do(ctx, "GET", fmt.Sprintf("/api/v1/websites/%s/", cfg.ID.ValueString()), nil, &out); err != nil {
		resp.Diagnostics.AddError("Could not read the website", err.Error())
		return
	}

	w := out.Website
	cfg.Domain = types.StringValue(w.Domain)
	cfg.Name = types.StringNull()
	if w.Name != nil {
		cfg.Name = types.StringValue(*w.Name)
	}
	cfg.WidgetID = types.StringValue(w.WidgetID)
	cfg.IsPrimary = types.BoolValue(w.IsPrimary)
	cfg.Ready = types.BoolValue(w.Ready)
	cfg.KnowledgeItems = types.Int64Value(w.KnowledgeItems)
	cfg.EmbedCode = types.StringValue(out.EmbedCode)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
