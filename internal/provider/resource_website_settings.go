package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &websiteSettingsResource{}
	_ resource.ResourceWithConfigure   = &websiteSettingsResource{}
	_ resource.ResourceWithImportState = &websiteSettingsResource{}
)

func NewWebsiteSettingsResource() resource.Resource { return &websiteSettingsResource{} }

type websiteSettingsResource struct{ client *Client }

type websiteSettingsModel struct {
	ID          types.String `tfsdk:"id"`
	WebsiteID   types.String `tfsdk:"website_id"`
	Settings    types.Map    `tfsdk:"settings"`
	AllSettings types.String `tfsdk:"all_settings_json"`
}

type settingsAPI struct {
	Settings map[string]interface{} `json:"settings"`
	Locked   map[string]interface{} `json:"locked"`
}

func (r *websiteSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website_settings"
}

func (r *websiteSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Widget settings of one website: the same settings as the Customize page of the " +
			"dashboard, for example `ai_support_name`, `widget_color` or `welcome_message`.\n\n" +
			"Terraform manages only the settings you list. Every other setting keeps its current value. " +
			"Write each value as text: `true`, `42` and `\"#0f172a\"` all work. For a list setting such as " +
			"`reply_languages`, use `jsonencode([\"en\", \"de\"])`.\n\n" +
			"Your plan decides which settings you can change. The API refuses a setting your plan cannot change, " +
			"and then saves nothing. `all_settings_json` shows every setting and its current value.\n\n" +
			"~> `terraform destroy` removes this resource from the Terraform state only. The settings keep their values.",
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
			"settings": schema.MapAttribute{
				MarkdownDescription: "The settings to manage, as `name = value`.",
				Required:            true,
				ElementType:         types.StringType,
			},
			"all_settings_json": schema.StringAttribute{
				MarkdownDescription: "Every setting of the website and its current value, as JSON. " +
					"Use `jsondecode()` to read it.",
				Computed: true,
			},
		},
	}
}

func (r *websiteSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *websiteSettingsResource) get(ctx context.Context, websiteID string) (*settingsAPI, error) {
	var out settingsAPI
	err := r.client.Do(ctx, "GET", fmt.Sprintf("/api/v1/websites/%s/settings/", websiteID), nil, &out)
	return &out, err
}

// toAPIValue turns the configured text into the JSON type the API holds for
// that setting today: a bool setting gets a bool, a number setting a number,
// a list setting the decoded list.
func toAPIValue(name, text string, current interface{}, known bool) (interface{}, error) {
	if !known {
		return text, nil
	}
	switch current.(type) {
	case bool:
		b, err := strconv.ParseBool(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("%s must be true or false, got %q", name, text)
		}
		return b, nil
	case float64:
		t := strings.TrimSpace(text)
		if i, err := strconv.ParseInt(t, 10, 64); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must be a number, got %q", name, text)
		}
		return f, nil
	case []interface{}:
		var list []interface{}
		if err := json.Unmarshal([]byte(text), &list); err != nil {
			return nil, fmt.Errorf("%s must be a list, written as jsonencode([...]), got %q", name, text)
		}
		return list, nil
	case nil:
		// Unset nullable setting: guess the type from the text itself.
		t := strings.TrimSpace(text)
		if t == "" {
			return nil, nil
		}
		return text, nil
	default:
		return text, nil
	}
}

// fromAPIValue writes an API value as the text a user would configure.
func fromAPIValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		buf, _ := json.Marshal(t)
		return string(buf)
	}
}

// sameSetting reports whether a configured text and an API value mean the
// same thing, so equal values never show as drift.
func sameSetting(configured string, apiValue interface{}) bool {
	got := fromAPIValue(apiValue)
	if configured == got {
		return true
	}
	switch apiValue.(type) {
	case bool:
		b, err := strconv.ParseBool(strings.TrimSpace(configured))
		return err == nil && b == apiValue.(bool)
	case float64:
		f, err := strconv.ParseFloat(strings.TrimSpace(configured), 64)
		return err == nil && f == apiValue.(float64)
	case []interface{}:
		var list interface{}
		if json.Unmarshal([]byte(configured), &list) != nil {
			return false
		}
		a, _ := json.Marshal(list)
		b, _ := json.Marshal(apiValue)
		return string(a) == string(b)
	case string:
		// The API trims text values.
		return strings.TrimSpace(configured) == apiValue.(string)
	}
	return false
}

func (r *websiteSettingsResource) apply(ctx context.Context, plan *websiteSettingsModel) error {
	websiteID := plan.WebsiteID.ValueString()

	wanted := map[string]string{}
	if d := plan.Settings.ElementsAs(ctx, &wanted, false); d.HasError() {
		return fmt.Errorf("could not read settings")
	}

	current, err := r.get(ctx, websiteID)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(wanted))
	for name := range wanted {
		names = append(names, name)
	}
	sort.Strings(names)

	var unknown []string
	body := map[string]interface{}{}
	for _, name := range names {
		value, known := current.Settings[name]
		if !known {
			unknown = append(unknown, name)
			continue
		}
		if sameSetting(wanted[name], value) {
			continue
		}
		v, err := toAPIValue(name, wanted[name], value, known)
		if err != nil {
			return err
		}
		body[name] = v
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown settings: %s. See all_settings_json for the names this website has", strings.Join(unknown, ", "))
	}

	if len(body) > 0 {
		if err := r.client.Do(ctx, "PATCH", fmt.Sprintf("/api/v1/websites/%s/settings/", websiteID), body, current); err != nil {
			return err
		}
	}

	all, _ := json.Marshal(current.Settings)
	plan.ID = plan.WebsiteID
	plan.AllSettings = types.StringValue(string(all))
	return nil
}

func (r *websiteSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan websiteSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Could not change the widget settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	websiteID := state.WebsiteID.ValueString()
	if websiteID == "" {
		websiteID = state.ID.ValueString()
	}
	current, err := r.get(ctx, websiteID)
	if IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read the widget settings", err.Error())
		return
	}

	managed := map[string]string{}
	if !state.Settings.IsNull() && !state.Settings.IsUnknown() {
		resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &managed, false)...)
	}
	refreshed := map[string]string{}
	for name, configured := range managed {
		value, ok := current.Settings[name]
		if !ok {
			continue // the setting no longer exists; drop it from state
		}
		if sameSetting(configured, value) {
			refreshed[name] = configured
		} else {
			refreshed[name] = fromAPIValue(value)
		}
	}

	settings, d := types.MapValueFrom(ctx, types.StringType, refreshed)
	resp.Diagnostics.Append(d...)
	all, _ := json.Marshal(current.Settings)

	state.ID = types.StringValue(websiteID)
	state.WebsiteID = types.StringValue(websiteID)
	state.Settings = settings
	state.AllSettings = types.StringValue(string(all))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan websiteSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Could not change the widget settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Nothing to call: the settings keep their values.
}

func (r *websiteSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// An imported resource manages no settings until you list them.
	empty, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{})
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("website_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("settings"), empty)...)
}
