resource "asyntai_website_settings" "shop" {
  website_id = asyntai_website.shop.id

  settings = {
    ai_support_name = "Ava"
    widget_color    = "#0f172a"
    use_emoji       = true
    reply_languages = jsonencode(["en", "de"])
  }
}
