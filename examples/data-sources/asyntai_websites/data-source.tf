data "asyntai_websites" "all" {}

output "domains" {
  value = [for w in data.asyntai_websites.all.websites : w.domain]
}
