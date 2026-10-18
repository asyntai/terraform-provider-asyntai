data "asyntai_website" "shop" {
  id = "42"
}

output "shop_embed_code" {
  value = data.asyntai_website.shop.embed_code
}
