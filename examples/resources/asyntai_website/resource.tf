resource "asyntai_website" "shop" {
  domain = "shop.example.com"
  name   = "Shop assistant"
}

# Put this snippet on the site.
output "embed_code" {
  value = asyntai_website.shop.embed_code
}
