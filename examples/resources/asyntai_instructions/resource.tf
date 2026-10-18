resource "asyntai_instructions" "shop" {
  website_id    = asyntai_website.shop.id
  ask_questions = true

  instructions = <<-EOT
    You are the shop assistant of Example Shop.
    Answer only from the shop's own pages and knowledge base.
    For order problems, ask for the order number and offer a support ticket.
  EOT
}
