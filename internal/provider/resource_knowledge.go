package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Two resources share this file: asyntai_knowledge_text and
// asyntai_knowledge_url. Both are knowledge base items. The API has no
// update call for an item, so every input forces a new item.

var (
	_ resource.Resource                = &knowledgeResource{}
	_ resource.ResourceWithConfigure   = &knowledgeResource{}
	_ resource.ResourceWithImportState = &knowledgeResource{}
)

func NewKnowledgeTextResource() resource.Resource { return &knowledgeResource{kind: "text"} }
func NewKnowledgeURLResource() resource.Resource  { return &knowledgeResource{kind: "url"} }

type knowledgeResource struct {
	client *Client
	kind   string // "text" or "url"
}

type knowledgeTextModel struct {
	ID          types.String `tfsdk:"id"`
	WebsiteID   types.String `tfsdk:"website_id"`
	Title       types.String `tfsdk:"title"`
	Content     types.String `tfsdk:"content"`
	AccessTags  types.List   `tfsdk:"access_tags"`
	ChunksCount types.Int64  `tfsdk:"chunks_count"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

type knowledgeURLModel struct {
	ID          types.String `tfsdk:"id"`
	WebsiteID   types.String `tfsdk:"website_id"`
	URL         types.String `tfsdk:"url"`
	Title       types.String `tfsdk:"title"`
	AccessTags  types.List   `tfsdk:"access_tags"`
	ChunksCount types.Int64  `tfsdk:"chunks_count"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

type knowledgeAPI struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	ChunksCount int64    `json:"chunks_count"`
	CreatedAt   string   `json:"created_at"`
	Content     *string  `json:"content"`
	AccessTags  []string `json:"access_tags"`
}

func (r *knowledgeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_knowledge_" + r.kind
}

func (r *knowledgeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replaceStr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	keepStr := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}

	common := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "Knowledge base item id.",
			Computed:            true,
			PlanModifiers:       keepStr,
		},
		"website_id": schema.StringAttribute{
			MarkdownDescription: "Id of the website the item belongs to. Defaults to the primary website of the account.",
			Optional:            true,
			Computed:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured(), stringplanmodifier.UseStateForUnknown()},
		},
		"access_tags": schema.ListAttribute{
			MarkdownDescription: "Access Tags (Enterprise plan): only visitors with one of these tags get answers from " +
				"this item. Leave out for everyone.",
			Optional:      true,
			ElementType:   types.StringType,
			PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
		},
		"chunks_count": schema.Int64Attribute{
			MarkdownDescription: "Number of search chunks Asyntai made from the item.",
			Computed:            true,
		},
		"created_at": schema.StringAttribute{
			MarkdownDescription: "When the item was added.",
			Computed:            true,
			PlanModifiers:       keepStr,
		},
	}

	if r.kind == "text" {
		common["title"] = schema.StringAttribute{
			MarkdownDescription: "Title of the item, for example `Opening hours`. Changing it creates a new item.",
			Required:            true,
			PlanModifiers:       replaceStr,
		}
		common["content"] = schema.StringAttribute{
			MarkdownDescription: "The text the AI agent answers from. At least 10 characters. Changing it creates a new item.",
			Required:            true,
			Validators:          []validator.String{stringvalidator.LengthAtLeast(10)},
			PlanModifiers:       replaceStr,
		}
		resp.Schema = schema.Schema{
			MarkdownDescription: "A text item in the knowledge base of a website, for example opening hours, a return " +
				"policy or product facts. The AI agent uses it to answer visitors.",
			Attributes: common,
		}
		return
	}

	common["url"] = schema.StringAttribute{
		MarkdownDescription: "Address of the page to add. Asyntai reads the page once, when the item is created. " +
			"Changing it creates a new item.",
		Required:      true,
		PlanModifiers: replaceStr,
	}
	common["title"] = schema.StringAttribute{
		MarkdownDescription: "Page title Asyntai read from the page.",
		Computed:            true,
		PlanModifiers:       keepStr,
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One web page in the knowledge base of a website. Asyntai reads the page when the item is " +
			"created. To read the page again, replace the item (`terraform apply -replace=...`).",
		Attributes: common,
	}
}

func (r *knowledgeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// primaryWebsiteID returns the id the API uses when no website_id is sent.
func (r *knowledgeResource) primaryWebsiteID(ctx context.Context) (string, error) {
	var out struct {
		Websites []struct {
			ID        int64 `json:"id"`
			IsPrimary bool  `json:"is_primary"`
		} `json:"websites"`
	}
	if err := r.client.Do(ctx, "GET", "/api/v1/websites/", nil, &out); err != nil {
		return "", err
	}
	for _, w := range out.Websites {
		if w.IsPrimary {
			return strconv.FormatInt(w.ID, 10), nil
		}
	}
	if len(out.Websites) > 0 {
		return strconv.FormatInt(out.Websites[0].ID, 10), nil
	}
	return "", fmt.Errorf("the account has no website yet")
}

func (r *knowledgeResource) get(ctx context.Context, id string) (*knowledgeAPI, error) {
	var out knowledgeAPI
	err := r.client.Do(ctx, "GET", fmt.Sprintf("/api/v1/knowledge/%s/", id), nil, &out)
	return &out, err
}

// itemWebsiteID finds the website an item belongs to. The single-item
// answer does not carry it, so the website lists are searched.
func (r *knowledgeResource) itemWebsiteID(ctx context.Context, id string) string {
	var sites struct {
		Websites []struct {
			ID int64 `json:"id"`
		} `json:"websites"`
	}
	if r.client.Do(ctx, "GET", "/api/v1/websites/", nil, &sites) != nil {
		return ""
	}
	for _, w := range sites.Websites {
		var list struct {
			Entries []struct {
				ID string `json:"id"`
			} `json:"entries"`
		}
		ws := strconv.FormatInt(w.ID, 10)
		if r.client.Do(ctx, "GET", "/api/v1/knowledge/?limit=100&website_id="+ws, nil, &list) != nil {
			continue
		}
		for _, e := range list.Entries {
			if e.ID == id {
				return ws
			}
		}
	}
	return ""
}

func (r *knowledgeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var websiteID types.String
	var accessTags types.List
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("website_id"), &websiteID)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("access_tags"), &accessTags)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wid := websiteID.ValueString()
	if websiteID.IsNull() || websiteID.IsUnknown() || wid == "" {
		id, err := r.primaryWebsiteID(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Could not find the primary website", err.Error())
			return
		}
		wid = id
	}

	body := map[string]interface{}{"website_id": wid}
	if !accessTags.IsNull() && !accessTags.IsUnknown() {
		var tags []string
		resp.Diagnostics.Append(accessTags.ElementsAs(ctx, &tags, false)...)
		body["access_tags"] = tags
	}

	var created struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}

	if r.kind == "text" {
		var plan knowledgeTextModel
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["title"] = plan.Title.ValueString()
		body["content"] = plan.Content.ValueString()
		if err := r.client.Do(ctx, "POST", "/api/v1/knowledge/text/", body, &created); err != nil {
			resp.Diagnostics.AddError("Could not add the text item", err.Error())
			return
		}
		item, err := r.get(ctx, created.ID)
		if err != nil {
			resp.Diagnostics.AddError("Text item added, but it could not be read back", err.Error())
			return
		}
		plan.ID = types.StringValue(created.ID)
		plan.WebsiteID = types.StringValue(wid)
		plan.ChunksCount = types.Int64Value(item.ChunksCount)
		plan.CreatedAt = types.StringValue(item.CreatedAt)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	var plan knowledgeURLModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body["url"] = plan.URL.ValueString()
	if err := r.client.Do(ctx, "POST", "/api/v1/knowledge/url/", body, &created); err != nil {
		resp.Diagnostics.AddError("Could not add the page", err.Error())
		return
	}
	item, err := r.get(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError("Page added, but it could not be read back", err.Error())
		return
	}
	plan.ID = types.StringValue(created.ID)
	plan.WebsiteID = types.StringValue(wid)
	plan.Title = types.StringValue(item.Title)
	plan.ChunksCount = types.Int64Value(item.ChunksCount)
	plan.CreatedAt = types.StringValue(item.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *knowledgeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var id, websiteID types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("website_id"), &websiteID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	item, err := r.get(ctx, id.ValueString())
	if IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the knowledge base item", err.Error())
		return
	}

	wid := websiteID.ValueString()
	if websiteID.IsNull() || wid == "" {
		wid = r.itemWebsiteID(ctx, id.ValueString())
	}

	var tags types.List
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("access_tags"), &tags)...)
	if len(item.AccessTags) > 0 || !tags.IsNull() {
		t, d := types.ListValueFrom(ctx, types.StringType, item.AccessTags)
		resp.Diagnostics.Append(d...)
		tags = t
	}

	if r.kind == "text" {
		var state knowledgeTextModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		state.Title = types.StringValue(item.Title)
		if item.Content != nil && (state.Content.IsNull() || !sameText(state.Content.ValueString(), *item.Content)) {
			state.Content = types.StringValue(*item.Content)
		}
		state.WebsiteID = types.StringValue(wid)
		state.AccessTags = tags
		state.ChunksCount = types.Int64Value(item.ChunksCount)
		state.CreatedAt = types.StringValue(item.CreatedAt)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	var state knowledgeURLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	state.Title = types.StringValue(item.Title)
	state.WebsiteID = types.StringValue(wid)
	state.AccessTags = tags
	state.ChunksCount = types.Int64Value(item.ChunksCount)
	state.CreatedAt = types.StringValue(item.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *knowledgeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every input forces a new item; nothing reaches Update except computed refreshes.
	resp.State.Raw = req.Plan.Raw
}

func (r *knowledgeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var id types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &id)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, "DELETE", fmt.Sprintf("/api/v1/knowledge/%s/", id.ValueString()), nil, nil)
	if err != nil && !IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete the knowledge base item", err.Error())
	}
}

func (r *knowledgeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if r.kind == "url" {
		// The API does not return the address of a page item, so an imported
		// item could never match its configuration.
		resp.Diagnostics.AddError("Import is not supported for asyntai_knowledge_url",
			"The Asyntai API does not return the address of a page item. Add the page again with Terraform, "+
				"then delete the old item in the dashboard.")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
