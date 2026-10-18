terraform {
  required_providers {
    asyntai = {
      source = "asyntai/asyntai"
    }
  }
}

# The API key comes from the ASYNTAI_API_KEY environment variable.
provider "asyntai" {}
