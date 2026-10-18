package provider

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func uniqueDomain(prefix string) string {
	return fmt.Sprintf("%s-%d.example.com", prefix, time.Now().UnixNano())
}

// ---- website + data sources --------------------------------------------------

func TestAccWebsite(t *testing.T) {
	domain := uniqueDomain("tf-site")
	cfg := fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  name   = "Terraform agent"
  crawl  = false
}

data "asyntai_website" "main" {
  id = asyntai_website.main.id
}

data "asyntai_websites" "all" {
  depends_on = [asyntai_website.main]
}
`, domain)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_website.main", "domain", domain),
					resource.TestCheckResourceAttr("asyntai_website.main", "name", "Terraform agent"),
					resource.TestCheckResourceAttr("asyntai_website.main", "is_primary", "false"),
					resource.TestMatchResourceAttr("asyntai_website.main", "widget_id", regexp.MustCompile(`^asyntai_`)),
					resource.TestMatchResourceAttr("asyntai_website.main", "embed_code", regexp.MustCompile(`<script`)),
					resource.TestCheckResourceAttrPair("data.asyntai_website.main", "widget_id", "asyntai_website.main", "widget_id"),
					resource.TestCheckResourceAttr("data.asyntai_website.main", "domain", domain),
					resource.TestCheckResourceAttr("data.asyntai_website.main", "ready", "true"),
					func(s *terraform.State) error {
						// The website really exists in the API.
						id, _ := attr(s, "asyntai_website.main", "id")
						var out struct {
							Website struct {
								Domain string `json:"domain"`
							} `json:"website"`
						}
						if err := apiGet("/api/v1/websites/"+id+"/", &out); err != nil {
							return err
						}
						if out.Website.Domain != domain {
							return fmt.Errorf("API has domain %q", out.Website.Domain)
						}
						// The list data source contains it.
						rs := s.RootModule().Resources["data.asyntai_websites.all"].Primary.Attributes
						for k, v := range rs {
							if strings.HasSuffix(k, ".domain") && v == domain {
								return nil
							}
						}
						return fmt.Errorf("asyntai_websites does not list %s", domain)
					},
				),
			},
			// Second apply of the same config: nothing to change.
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            "asyntai_website.main",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"crawl", "allow_duplicate"},
			},
			// A new domain replaces the website.
			{
				Config: strings.Replace(cfg, domain, "x"+domain, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("asyntai_website.main", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.TestCheckResourceAttr("asyntai_website.main", "domain", "x"+domain),
			},
		},
	})
}

func TestAccWebsiteDuplicateRefused(t *testing.T) {
	domain := uniqueDomain("tf-dup")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "asyntai_website" "a" {
  domain = %q
  crawl  = false
}
resource "asyntai_website" "b" {
  domain     = asyntai_website.a.domain
  crawl      = false
  depends_on = [asyntai_website.a]
}
`, domain),
				ExpectError: regexp.MustCompile(`already has`),
			},
		},
	})
}

// ---- instructions ------------------------------------------------------------

func TestAccInstructions(t *testing.T) {
	domain := uniqueDomain("tf-instr")
	long := strings.Repeat("Answer politely and briefly. ", 20)
	cfg := func(text string, shrink bool) string {
		return fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}

resource "asyntai_instructions" "main" {
  website_id    = asyntai_website.main.id
  mode          = "instructions"
  ask_questions = false
  allow_shrink  = %t
  instructions  = <<-EOT
    %s
  EOT
}
`, domain, shrink, text)
	}

	readBack := func(want string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			id, _ := attr(s, "asyntai_website.main", "id")
			var out struct {
				Instructions string `json:"instructions"`
				AskQuestions bool   `json:"ask_questions"`
			}
			if err := apiGet("/api/v1/websites/"+id+"/instructions/", &out); err != nil {
				return err
			}
			if out.Instructions != strings.TrimSpace(want) {
				return fmt.Errorf("API holds %q", out.Instructions)
			}
			if out.AskQuestions {
				return fmt.Errorf("ask_questions is still true in the API")
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg(long, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_instructions.main", "mode", "instructions"),
					resource.TestCheckResourceAttr("asyntai_instructions.main", "version", "1"),
					readBack(long),
				),
			},
			// The heredoc's trailing newline must not show as drift.
			{
				Config: cfg(long, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg(long+" Always mention the opening hours.", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_instructions.main", "version", "2"),
					readBack(long+" Always mention the opening hours."),
				),
			},
			// Cutting the text below half is refused by the API...
			{
				Config:      cfg("Be brief.", false),
				ExpectError: regexp.MustCompile(`would cut the instructions`),
			},
			// ...unless allow_shrink is set.
			{
				Config: cfg("Be brief.", true),
				Check:  readBack("Be brief."),
			},
			{
				ResourceName:            "asyntai_instructions.main",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"allow_shrink", "instructions"},
			},
		},
	})
}

// ---- widget settings -----------------------------------------------------------

func TestAccWebsiteSettings(t *testing.T) {
	domain := uniqueDomain("tf-settings")
	cfg := func(name, color string, emoji bool, delay int, langs string) string {
		return fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}

resource "asyntai_website_settings" "main" {
  website_id = asyntai_website.main.id
  settings = {
    ai_support_name    = %q
    widget_color       = %q
    use_emoji          = %t
    auto_trigger_delay = %d
    reply_languages    = jsonencode(%s)
  }
}
`, domain, name, color, emoji, delay, langs)
	}

	apiSettings := func(check func(map[string]interface{}) error) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			id, _ := attr(s, "asyntai_website.main", "id")
			var out struct {
				Settings map[string]interface{} `json:"settings"`
			}
			if err := apiGet("/api/v1/websites/"+id+"/settings/", &out); err != nil {
				return err
			}
			return check(out.Settings)
		}
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg("Ava", "#0f172a", true, 12, `["en", "de"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_website_settings.main", "settings.ai_support_name", "Ava"),
					resource.TestCheckResourceAttr("asyntai_website_settings.main", "settings.use_emoji", "true"),
					apiSettings(func(m map[string]interface{}) error {
						if m["ai_support_name"] != "Ava" || m["widget_color"] != "#0f172a" ||
							m["use_emoji"] != true || m["auto_trigger_delay"] != float64(12) {
							return fmt.Errorf("API settings not applied: %v %v %v %v",
								m["ai_support_name"], m["widget_color"], m["use_emoji"], m["auto_trigger_delay"])
						}
						langs, _ := json.Marshal(m["reply_languages"])
						if string(langs) != `["en","de"]` {
							return fmt.Errorf("reply_languages is %s", langs)
						}
						// An unmanaged setting keeps its default.
						if m["message_character_limit"] != float64(750) {
							return fmt.Errorf("unmanaged setting changed: %v", m["message_character_limit"])
						}
						return nil
					}),
					func(s *terraform.State) error {
						all, _ := attr(s, "asyntai_website_settings.main", "all_settings_json")
						if !strings.Contains(all, `"message_character_limit"`) {
							return fmt.Errorf("all_settings_json misses settings")
						}
						return nil
					},
				),
			},
			{
				Config: cfg("Ava", "#0f172a", true, 12, `["en", "de"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: cfg("Max", "#ffffff", false, 3, `["fr"]`),
				Check: apiSettings(func(m map[string]interface{}) error {
					if m["ai_support_name"] != "Max" || m["use_emoji"] != false || m["auto_trigger_delay"] != float64(3) {
						return fmt.Errorf("update not applied: %v %v %v", m["ai_support_name"], m["use_emoji"], m["auto_trigger_delay"])
					}
					return nil
				}),
			},
		},
	})
}

func TestAccWebsiteSettingsErrors(t *testing.T) {
	domain := uniqueDomain("tf-settings-err")
	base := `
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}
resource "asyntai_website_settings" "main" {
  website_id = asyntai_website.main.id
  settings   = { %s }
}
`
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(base, domain, `no_such_setting = "x"`),
				ExpectError: regexp.MustCompile(`unknown settings: no_such_setting`),
			},
			{
				Config:      fmt.Sprintf(base, domain, `use_emoji = "maybe"`),
				ExpectError: regexp.MustCompile(`use_emoji must be true or false`),
			},
			{
				Config:      fmt.Sprintf(base, domain, `widget_color = "dark blue"`),
				ExpectError: regexp.MustCompile(`hex colour`),
			},
		},
	})
}

// ---- knowledge -----------------------------------------------------------------

func TestAccKnowledgeText(t *testing.T) {
	domain := uniqueDomain("tf-kb")
	cfg := func(content string) string {
		return fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}

resource "asyntai_knowledge_text" "hours" {
  website_id = asyntai_website.main.id
  title      = "Opening hours"
  content    = %q
}
`, domain, content)
	}

	var firstID string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			for _, rs := range s.RootModule().Resources {
				if rs.Type != "asyntai_knowledge_text" {
					continue
				}
				err := apiGet("/api/v1/knowledge/"+rs.Primary.ID+"/", &struct{}{})
				if !IsNotFound(err) {
					return fmt.Errorf("item %s still exists after destroy (err=%v)", rs.Primary.ID, err)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg("We are open Monday to Friday, 9 to 17."),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_knowledge_text.hours", "title", "Opening hours"),
					resource.TestCheckResourceAttrPair("asyntai_knowledge_text.hours", "website_id", "asyntai_website.main", "id"),
					func(s *terraform.State) error {
						firstID, _ = attr(s, "asyntai_knowledge_text.hours", "id")
						var out struct {
							Content string `json:"content"`
						}
						if err := apiGet("/api/v1/knowledge/"+firstID+"/", &out); err != nil {
							return err
						}
						if out.Content != "We are open Monday to Friday, 9 to 17." {
							return fmt.Errorf("API content %q", out.Content)
						}
						return nil
					},
				),
			},
			{
				Config: cfg("We are open Monday to Friday, 9 to 17."),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:      "asyntai_knowledge_text.hours",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// New content: a new item replaces the old one, and the old one is gone.
			{
				Config: cfg("We are open every day, 8 to 20."),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("asyntai_knowledge_text.hours", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: func(s *terraform.State) error {
					newID, _ := attr(s, "asyntai_knowledge_text.hours", "id")
					if newID == firstID {
						return fmt.Errorf("id did not change")
					}
					if err := apiGet("/api/v1/knowledge/"+firstID+"/", &struct{}{}); !IsNotFound(err) {
						return fmt.Errorf("old item %s still exists (err=%v)", firstID, err)
					}
					return nil
				},
			},
		},
	})
}

func TestAccKnowledgeTextPrimaryWebsite(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// No website_id: the item goes to the primary website.
				Config: `
data "asyntai_websites" "all" {}

resource "asyntai_knowledge_text" "faq" {
  title   = "Shipping"
  content = "We ship to all EU countries within 3 days."
}
`,
				Check: func(s *terraform.State) error {
					wid, _ := attr(s, "asyntai_knowledge_text.faq", "website_id")
					ds := s.RootModule().Resources["data.asyntai_websites.all"].Primary.Attributes
					for k, v := range ds {
						if strings.HasSuffix(k, ".is_primary") && v == "true" {
							if ds[strings.TrimSuffix(k, ".is_primary")+".id"] == wid {
								return nil
							}
						}
					}
					return fmt.Errorf("website_id %s is not the primary website", wid)
				},
			},
			{
				Config: `
resource "asyntai_knowledge_text" "faq" {
  title   = "Shipping"
  content = "We ship to all EU countries within 3 days."
}
`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccKnowledgeURL(t *testing.T) {
	domain := uniqueDomain("tf-kburl")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}

resource "asyntai_knowledge_url" "page" {
  website_id = asyntai_website.main.id
  url        = "https://example.com/"
}
`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("asyntai_knowledge_url.page", "title", "Example Domain"),
					resource.TestCheckResourceAttrSet("asyntai_knowledge_url.page", "id"),
				),
			},
			{
				ResourceName: "asyntai_knowledge_url.page",
				ImportState:  true,
				ExpectError:  regexp.MustCompile(`Import is not supported`),
			},
		},
	})
}

// ---- webhooks ------------------------------------------------------------------

func TestAccWebhook(t *testing.T) {
	domain := uniqueDomain("tf-hook")
	cfg := func(events string) string {
		return fmt.Sprintf(`
resource "asyntai_website" "main" {
  domain = %q
  crawl  = false
}

resource "asyntai_webhook" "leads" {
  website_id = asyntai_website.main.id
  url        = "https://hooks.example.com/asyntai"
  events     = %s
}
`, domain, events)
	}

	var firstID string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			var out struct {
				Webhooks []struct {
					ID string `json:"id"`
				} `json:"webhooks"`
			}
			if err := apiGet("/api/v1/webhooks/", &out); err != nil {
				return err
			}
			for _, rs := range s.RootModule().Resources {
				for _, w := range out.Webhooks {
					if rs.Type == "asyntai_webhook" && w.ID == rs.Primary.ID {
						return fmt.Errorf("webhook %s still exists", w.ID)
					}
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: cfg(`["lead.captured", "ticket.created"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("asyntai_webhook.leads", "secret", regexp.MustCompile(`^whsec_`)),
					resource.TestCheckResourceAttr("asyntai_webhook.leads", "events.#", "2"),
					resource.TestCheckResourceAttr("asyntai_webhook.leads", "is_active", "true"),
					func(s *terraform.State) error {
						firstID, _ = attr(s, "asyntai_webhook.leads", "id")
						return nil
					},
				),
			},
			{
				Config: cfg(`["ticket.created", "lead.captured"]`), // same set, other order
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				ResourceName:            "asyntai_webhook.leads",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"secret", "website_id"},
			},
			{
				Config: cfg(`["lead.captured"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("asyntai_webhook.leads", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: func(s *terraform.State) error {
					newID, _ := attr(s, "asyntai_webhook.leads", "id")
					if newID == firstID {
						return fmt.Errorf("id did not change")
					}
					return nil
				},
			},
		},
	})
}

func TestAccWebhookBadEvent(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
resource "asyntai_webhook" "bad" {
  url    = "https://hooks.example.com/asyntai"
  events = ["no.such.event"]
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)value must be one of`),
			},
			{
				Config: `
resource "asyntai_webhook" "bad" {
  url    = "ftp://hooks.example.com/asyntai"
  events = ["lead.captured"]
}
`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must start with http`),
			},
		},
	})
}
