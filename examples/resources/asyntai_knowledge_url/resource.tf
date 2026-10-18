resource "asyntai_knowledge_url" "pricing" {
  website_id = asyntai_website.shop.id
  url        = "https://shop.example.com/pricing"
}
