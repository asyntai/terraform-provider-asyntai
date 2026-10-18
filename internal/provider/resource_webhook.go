package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &webhookResource{}
	_ resource.ResourceWithConfigure   = &webhookResource{}
	_ resource.ResourceWithImportState = &webhookResource{}
)

// The events the API accepts (VALID_WEBHOOK_EVENTS in the API).
var webhookEvents = []string{
	"message.received", "conversation.started", "escalation.requested",
	"takeover.started", "lead.captured", "ticket.created",
}

func NewWebhookResource() resource.Resource { return &webhookResource{} }

type webhookResource struct{ client *Client }

type webhookModel struct {
	ID        types.String `tfsdk:"id"`
	WebsiteID types.String `tfsdk:"website_id"`
	URL       types.String `tfsdk:"url"`
	Events    types.Set    `tfsdk:"events"`
	Secret    types.String `tfsdk:"secret"`
	IsActive  types.Bool   `tfsdk:"is_active"`
	CreatedAt types.String `tfsdk:"created_at"`
}

type webhookAPI struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret"`
	IsActive  bool     `json:"is_active"`
	CreatedAt string   `json:"created_at"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A webhook: Asyntai sends a signed POST request to your address when an event happens, " +
			"for example a new chat or a captured lead. The API has no update call for webhooks, so a change creates " +
			"a new webhook with a new secret.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Webhook id.",
				Computed:            true,
				PlanModifiers:       keep,
			},
			"website_id": schema.StringAttribute{
				MarkdownDescription: "Id of the website whose events are sent. Defaults to the primary website.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "Address that receives the events. Must start with `http://` or `https://`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(httpURL, "must start with http:// or https://")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"events": schema.SetAttribute{
				MarkdownDescription: "Events to send: `message.received`, `conversation.started`, `escalation.requested`, " +
					"`takeover.started`, `lead.captured`, `ticket.created`.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(webhookEvents...)),
				},
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"secret": schema.StringAttribute{
				MarkdownDescription: "Signing secret (`whsec_...`). Use it to check that a request comes from Asyntai. " +
					"The API shows it only once, at create; an imported webhook has no secret in state.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: keep,
			},
			"is_active": schema.BoolAttribute{
				MarkdownDescription: "Whether Asyntai still sends events to the address.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				MarkdownDescription: "When the webhook was created.",
				Computed:            true,
				PlanModifiers:       keep,
			},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var events []string
	resp.Diagnostics.Append(plan.Events.ElementsAs(ctx, &events, false)...)
	body := map[string]interface{}{"url": plan.URL.ValueString(), "events": events}
	if !plan.WebsiteID.IsNull() {
		body["website_id"] = plan.WebsiteID.ValueString()
	}

	var out struct {
		Webhook webhookAPI `json:"webhook"`
	}
	if err := r.client.Do(ctx, "POST", "/api/v1/webhooks/", body, &out); err != nil {
		resp.Diagnostics.AddError("Could not create the webhook", err.Error())
		return
	}

	plan.ID = types.StringValue(out.Webhook.ID)
	plan.Secret = types.StringValue(out.Webhook.Secret)
	plan.IsActive = types.BoolValue(true)
	plan.CreatedAt = types.StringValue(out.Webhook.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) find(ctx context.Context, id string) (*webhookAPI, error) {
	var out struct {
		Webhooks []webhookAPI `json:"webhooks"`
	}
	if err := r.client.Do(ctx, "GET", "/api/v1/webhooks/", nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Webhooks {
		if out.Webhooks[i].ID == id {
			return &out.Webhooks[i], nil
		}
	}
	return nil, nil
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hook, err := r.find(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Could not read the webhooks", err.Error())
		return
	}
	if hook == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	events, d := types.SetValueFrom(ctx, types.StringType, hook.Events)
	resp.Diagnostics.Append(d...)
	state.URL = types.StringValue(hook.URL)
	state.Events = events
	state.IsActive = types.BoolValue(hook.IsActive)
	state.CreatedAt = types.StringValue(hook.CreatedAt)
	if state.Secret.IsUnknown() {
		state.Secret = types.StringNull()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every input forces a new webhook; nothing reaches Update except computed refreshes.
	resp.State.Raw = req.Plan.Raw
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, "DELETE", fmt.Sprintf("/api/v1/webhooks/%s/", state.ID.ValueString()), nil, nil)
	if err != nil && !IsNotFound(err) {
		resp.Diagnostics.AddError("Could not delete the webhook", err.Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
