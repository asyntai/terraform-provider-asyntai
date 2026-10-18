package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &instructionsResource{}
	_ resource.ResourceWithConfigure   = &instructionsResource{}
	_ resource.ResourceWithImportState = &instructionsResource{}
)

func NewInstructionsResource() resource.Resource { return &instructionsResource{} }

type instructionsResource struct{ client *Client }

type instructionsModel struct {
	ID           types.String `tfsdk:"id"`
	WebsiteID    types.String `tfsdk:"website_id"`
	Instructions types.String `tfsdk:"instructions"`
	Mode         types.String `tfsdk:"mode"`
	AskQuestions types.Bool   `tfsdk:"ask_questions"`
	AllowShrink  types.Bool   `tfsdk:"allow_shrink"`
	Version      types.Int64  `tfsdk:"version"`
}

type instructionsAPI struct {
	Instructions string `json:"instructions"`
	Version      int64  `json:"version"`
	Mode         string `json:"mode"`
	AskQuestions bool   `json:"ask_questions"`
}

func (r *instructionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instructions"
}

func (r *instructionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The AI instructions of one website: the rules the AI agent follows when it answers. " +
			"Terraform owns the whole text. Each change keeps the previous text in the instruction history of the " +
			"dashboard.\n\n" +
			"~> `terraform destroy` removes this resource from the Terraform state only. The instructions stay on the " +
			"website, because an agent without instructions cannot answer.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `website_id`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"website_id": schema.StringAttribute{
				MarkdownDescription: "Id of the website, for example `asyntai_website.main.id`.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"instructions": schema.StringAttribute{
				MarkdownDescription: "The full instruction text. Surrounding white space is ignored.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "`instructions` (follow these rules) or `general` (general assistant). " +
					"Defaults to the current mode of the website.",
				Optional:      true,
				Computed:      true,
				Validators:    []validator.String{stringvalidator.OneOf("instructions", "general")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ask_questions": schema.BoolAttribute{
				MarkdownDescription: "Let the agent ask the visitor short follow-up questions. " +
					"Defaults to the current value of the website.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"allow_shrink": schema.BoolAttribute{
				MarkdownDescription: "The API refuses a write that cuts the text to less than half of its current " +
					"length, to protect against an empty or cut-off value. Set `true` to allow it. Defaults to `false`.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"version": schema.Int64Attribute{
				MarkdownDescription: "Version number of the instructions. It goes up by one on each change.",
				Computed:            true,
			},
		},
	}
}

func (r *instructionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *instructionsResource) write(ctx context.Context, plan *instructionsModel) error {
	body := map[string]interface{}{
		"instructions": plan.Instructions.ValueString(),
		"force":        plan.AllowShrink.ValueBool(),
	}
	if !plan.Mode.IsNull() && !plan.Mode.IsUnknown() {
		body["mode"] = plan.Mode.ValueString()
	}
	if !plan.AskQuestions.IsNull() && !plan.AskQuestions.IsUnknown() {
		body["ask_questions"] = plan.AskQuestions.ValueBool()
	}

	var out struct {
		Version      int64  `json:"version"`
		Mode         string `json:"mode"`
		AskQuestions bool   `json:"ask_questions"`
	}
	err := r.client.Do(ctx, "PUT", fmt.Sprintf("/api/v1/websites/%s/instructions/", plan.WebsiteID.ValueString()), body, &out)
	if err != nil {
		return err
	}
	plan.ID = plan.WebsiteID
	plan.Version = types.Int64Value(out.Version)
	plan.Mode = types.StringValue(out.Mode)
	plan.AskQuestions = types.BoolValue(out.AskQuestions)
	return nil
}

func (r *instructionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan instructionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Could not write the AI instructions", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instructionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state instructionsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	websiteID := state.WebsiteID.ValueString()
	if websiteID == "" {
		websiteID = state.ID.ValueString()
	}

	var out instructionsAPI
	err := r.client.Do(ctx, "GET", fmt.Sprintf("/api/v1/websites/%s/instructions/", websiteID), nil, &out)
	if IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the AI instructions", err.Error())
		return
	}

	state.ID = types.StringValue(websiteID)
	state.WebsiteID = types.StringValue(websiteID)
	// Keep the configured spelling when only surrounding white space differs.
	if state.Instructions.IsNull() || !sameText(state.Instructions.ValueString(), out.Instructions) {
		state.Instructions = types.StringValue(out.Instructions)
	}
	state.Mode = types.StringValue(out.Mode)
	state.AskQuestions = types.BoolValue(out.AskQuestions)
	state.Version = types.Int64Value(out.Version)
	if state.AllowShrink.IsNull() {
		state.AllowShrink = types.BoolValue(false)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *instructionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan instructionsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Could not write the AI instructions", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instructionsResource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Nothing to call: the instructions stay on the website.
}

func (r *instructionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("website_id"), req.ID)...)
}
