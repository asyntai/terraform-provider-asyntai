# Terraform Provider for Asyntai AI Chatbot

Manage your [Asyntai](https://asyntai.com) AI chatbots as code. Create AI agents for your websites, set the widget look and behaviour, write the AI instructions, fill the knowledge base and register webhooks, all from Terraform.

Agencies and teams that run chatbots on many sites keep every bot in version control, review changes in pull requests and roll out the same setup to each new site with one `terraform apply`.

Full guide: [asyntai.com/documentation/integrations/terraform/](https://asyntai.com/documentation/integrations/terraform/)

## Example

```hcl
terraform {
  required_providers {
    asyntai = {
      source = "asyntai/asyntai"
    }
  }
}

# The API key comes from the ASYNTAI_API_KEY environment variable.
provider "asyntai" {}

resource "asyntai_website" "shop" {
  domain = "shop.example.com"
  name   = "Shop assistant"
}

resource "asyntai_website_settings" "shop" {
  website_id = asyntai_website.shop.id
  settings = {
    ai_support_name = "Ava"
    widget_color    = "#0f172a"
    use_emoji       = true
  }
}

resource "asyntai_instructions" "shop" {
  website_id   = asyntai_website.shop.id
  instructions = <<-EOT
    You are the shop assistant of Example Shop.
    For order problems, ask for the order number and offer a support ticket.
  EOT
}

resource "asyntai_knowledge_text" "returns" {
  website_id = asyntai_website.shop.id
  title      = "Return policy"
  content    = "You can return any item within 30 days. Returns are free inside the EU."
}

resource "asyntai_webhook" "leads" {
  website_id = asyntai_website.shop.id
  url        = "https://hooks.example.com/asyntai"
  events     = ["lead.captured", "ticket.created"]
}

output "embed_code" {
  value = asyntai_website.shop.embed_code
}
```

## Resources and data sources

| Name | What it manages |
| --- | --- |
| `asyntai_website` | A website (AI agent). Gives the widget id and the embed code. |
| `asyntai_website_settings` | Widget settings: name, colours, welcome message, languages and more. |
| `asyntai_instructions` | The AI instructions the agent follows. |
| `asyntai_knowledge_text` | A text item in the knowledge base. |
| `asyntai_knowledge_url` | A web page in the knowledge base. |
| `asyntai_webhook` | A webhook for new chats, leads, tickets and more. |
| `data.asyntai_websites` | All websites of the account. |
| `data.asyntai_website` | One website: widget id, embed code, setup status. |

## Authentication

Copy your API key from the [API Settings](https://asyntai.com/settings/api/) page of Asyntai and export it:

```sh
export ASYNTAI_API_KEY="your-key"
```

The API is part of the Starter, Standard and Pro plans.

## OpenTofu

The provider works with OpenTofu 1.6 or later, from the OpenTofu Registry under the same address. Use the same configuration and run `tofu init` and `tofu apply`.

OpenTofu guide: [asyntai.com/documentation/integrations/opentofu/](https://asyntai.com/documentation/integrations/opentofu/)

## Development

Requirements: Go 1.25 or newer, Terraform 1.0 or newer or OpenTofu 1.6 or newer.

```sh
go build ./...
go test ./...        # unit tests
go generate ./...    # regenerate docs/ from the schema and examples/
```

Acceptance tests run real Terraform against an Asyntai API. Run them against a local test server, never against a real account:

```sh
TF_ACC=1 ASYNTAI_BASE_URL=http://127.0.0.1:8015 ASYNTAI_API_KEY=... go test ./... -v
```

To run them with OpenTofu, point the test framework at `tofu` and give it the OpenTofu registry host:

```sh
TF_ACC=1 TF_ACC_TERRAFORM_PATH=$(which tofu) TF_ACC_PROVIDER_HOST=registry.opentofu.org TF_ACC_PROVIDER_NAMESPACE=asyntai \
  ASYNTAI_BASE_URL=http://127.0.0.1:8015 ASYNTAI_API_KEY=... go test ./... -v
```

## License

[MPL-2.0](LICENSE)
