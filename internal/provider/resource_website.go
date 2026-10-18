package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &websiteResource{}
	_ resource.ResourceWithConfigure   = &websiteResource{}
	_ resource.ResourceWithImportState = &websiteResource{}
)

func NewWebsiteResource() resource.Resource { return &websiteResource{} }

type websiteResource struct{ client *Client }

type websiteModel struct {
	ID        types.String `tfsdk:"id"`
	Domain    types.String `tfsdk:"domain"`
	Name      types.String `tfsdk:"name"`
	Crawl     types.Bool   `tfsdk:"crawl"`
	Force     types.Bool   `tfsdk:"allow_duplicate"`
	WidgetID  types.String `tfsdk:"widget_id"`
	IsPrimary types.Bool   `tfsdk:"is_primary"`
	EmbedCode types.String `tfsdk:"embed_code"`
}

// websiteAPI is the website object the API returns.
type websiteAPI struct {
	ID        int64   `json:"id"`
	Domain    string  `json:"domain"`
	Name      *string `json:"name"`
	WidgetID  string  `json:"widget_id"`
	IsPrimary bool    `json:"is_primary"`
}

func (r *websiteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website"
}

func (r *websiteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A website (AI agent) in your Asyntai account. On create, Asyntai reads the site and " +
			"writes a first draft of the AI instructions, the same as when you add a website in the dashboard.\n\n" +
			"~> The API has no delete call for websites. `terraform destroy` removes the website from the Terraform " +
			"state only; the website and its chat history stay in your Asyntai account. Delete it in the dashboard if you " +
			"want it gone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Website id.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Domain of the website, for example `example.com`. Changing it creates a new website.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name of the agent in the dashboard. Changing it creates a new website.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"crawl": schema.BoolAttribute{
				MarkdownDescription: "Read the site and write the first AI instructions on create. Defaults to `true`. " +
					"Only used when the website is created.",
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"allow_duplicate": schema.BoolAttribute{
				MarkdownDescription: "Create the website even when the account already has the same domain, as a second " +
					"agent. Defaults to `false`.",
				Optional:      true,
				Computed:      true,
				Default:       booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"widget_id": schema.StringAttribute{
				MarkdownDescription: "Widget id, used in the embed code.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_primary": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the primary website of the account.",
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"embed_code": schema.StringAttribute{
				MarkdownDescription: "The `<script>` snippet to put on the site.",
				Computed:            true,
			},
		},
	}
}

func (r *websiteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *websiteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan websiteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]interface{}{
		"domain": plan.Domain.ValueString(),
		"crawl":  plan.Crawl.ValueBool(),
		"force":  plan.Force.ValueBool(),
	}
	if !plan.Name.IsNull() {
		body["name"] = plan.Name.ValueString()
	}

	var out struct {
		Website   websiteAPI `json:"website"`
		EmbedCode string     `json:"embed_code"`
	}
	if err := r.client.Do(ctx, "POST", "/api/v1/websites/", body, &out); err != nil {
		resp.Diagnostics.AddError("Could not create the website", err.Error())
		return
	}

	plan.ID = types.StringValue(strconv.FormatInt(out.Website.ID, 10))
	plan.WidgetID = types.StringValue(out.Website.WidgetID)
	plan.IsPrimary = types.BoolValue(out.Website.IsPrimary)
	plan.EmbedCode = types.StringValue(out.EmbedCode)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var out struct {
		Website   websiteAPI `json:"website"`
		EmbedCode string     `json:"embed_code"`
	}
	err := r.client.Do(ctx, "GET", fmt.Sprintf("/api/v1/websites/%s/", state.ID.ValueString()), nil, &out)
	if IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the website", err.Error())
		return
	}

	// The API shows "example.com (2)" for a second copy of a domain. Keep the
	// configured domain so that does not look like drift.
	if state.Domain.IsNull() || state.Domain.ValueString() == "" {
		state.Domain = types.StringValue(out.Website.Domain)
	}
	if out.Website.Name != nil && *out.Website.Name != "" {
		state.Name = types.StringValue(*out.Website.Name)
	} else if !state.Name.IsNull() && state.Name.ValueString() != "" {
		state.Name = types.StringNull()
	}
	if state.Crawl.IsNull() {
		state.Crawl = types.BoolValue(true)
	}
	if state.Force.IsNull() {
		state.Force = types.BoolValue(false)
	}
	state.WidgetID = types.StringValue(out.Website.WidgetID)
	state.IsPrimary = types.BoolValue(out.Website.IsPrimary)
	state.EmbedCode = types.StringValue(out.EmbedCode)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every user-settable attribute forces a new resource, so an update only
	// refreshes computed values.
	var plan websiteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state websiteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.AddWarning("Website kept in your Asyntai account",
		fmt.Sprintf("The Asyntai API cannot delete websites, so %s (id %s) was only removed from the Terraform state. "+
			"Delete it in the Asyntai dashboard if you no longer need it.", state.Domain.ValueString(), state.ID.ValueString()))
}

func (r *websiteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
