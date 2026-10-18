resource "asyntai_webhook" "leads" {
  website_id = asyntai_website.shop.id
  url        = "https://hooks.example.com/asyntai"
  events     = ["lead.captured", "ticket.created"]
}

output "webhook_secret" {
  value     = asyntai_webhook.leads.secret
  sensitive = true
}
