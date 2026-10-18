resource "asyntai_knowledge_text" "returns" {
  website_id = asyntai_website.shop.id
  title      = "Return policy"
  content    = "You can return any item within 30 days. Returns are free inside the EU."
}

# Keep longer texts in files next to your configuration.
resource "asyntai_knowledge_text" "faq" {
  website_id = asyntai_website.shop.id
  title      = "Shipping FAQ"
  content    = file("${path.module}/shipping-faq.md")
}
